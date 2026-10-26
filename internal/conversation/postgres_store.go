package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SkyC0der/skyroom/internal/permission"
	platformid "github.com/SkyC0der/skyroom/internal/platform/id"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) HasRoomCapability(ctx context.Context, userID, roomID string, capability permission.Capability) (bool, error) {
	var allowed bool
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(bit_or(role.permissions), 0) & $3 <> 0
		FROM rooms room
		JOIN members member ON member.community_id = room.community_id AND member.user_id = $1 AND member.state = 'active'
		JOIN member_roles assignment ON assignment.member_id = member.id AND assignment.community_id = member.community_id
		JOIN roles role ON role.id = assignment.role_id AND role.community_id = assignment.community_id
		WHERE room.id = $2`, userID, roomID, int64(capability)).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("authorize room capability: %w", err)
	}
	return allowed, nil
}

func (s *PostgresStore) CreateMessage(ctx context.Context, userID, authorHandle, roomID, idempotencyKey, body string, now time.Time) (PostResult, error) {
	messageID, err := platformid.New()
	if err != nil {
		return PostResult{}, err
	}
	eventID, err := platformid.New()
	if err != nil {
		return PostResult{}, err
	}
	var result PostResult
	result.EventID = eventID
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var memberID string
		if err := tx.QueryRow(ctx, `
			SELECT room.community_id::text, member.id::text
			FROM rooms room
			JOIN members member ON member.community_id = room.community_id
			WHERE room.id = $1 AND member.user_id = $2 AND member.state = 'active'
			FOR SHARE OF room, member`, roomID, userID,
		).Scan(&result.Message.CommunityID, &memberID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotAuthorized
			}
			return fmt.Errorf("resolve message author: %w", err)
		}

		err := tx.QueryRow(ctx, `
			INSERT INTO messages (id, community_id, room_id, author_member_id, client_idempotency_key, body, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (author_member_id, client_idempotency_key) DO NOTHING
			RETURNING id::text, community_id::text, room_id::text, client_idempotency_key, body, revision, created_at, edited_at, deleted_at`,
			messageID, result.Message.CommunityID, roomID, memberID, idempotencyKey, body, now,
		).Scan(&result.Message.ID, &result.Message.CommunityID, &result.Message.RoomID, &result.Message.ClientIdempotencyKey, &result.Message.Body, &result.Message.Revision, &result.Message.CreatedAt, &result.Message.EditedAt, &result.Message.DeletedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			result.Duplicate = true
			var initialBody string
			if err := tx.QueryRow(ctx, `
				SELECT message.id::text, message.community_id::text, message.room_id::text,
				       app_user.id::text, app_user.handle, message.client_idempotency_key,
			       message.body, message.revision, message.created_at, message.edited_at, message.deleted_at,
			       initial_revision.body
				FROM messages message
				JOIN members author ON author.id = message.author_member_id
				JOIN users app_user ON app_user.id = author.user_id
				JOIN message_revisions initial_revision ON initial_revision.message_id = message.id AND initial_revision.revision = 1
				WHERE message.author_member_id = $1 AND message.client_idempotency_key = $2`, memberID, idempotencyKey,
			).Scan(&result.Message.ID, &result.Message.CommunityID, &result.Message.RoomID, &result.Message.AuthorUserID, &result.Message.AuthorHandle, &result.Message.ClientIdempotencyKey, &result.Message.Body, &result.Message.Revision, &result.Message.CreatedAt, &result.Message.EditedAt, &result.Message.DeletedAt, &initialBody); err != nil {
				return fmt.Errorf("load idempotent message: %w", err)
			}
			if result.Message.RoomID != roomID || initialBody != body {
				return ErrIdempotencyConflict
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("insert message: %w", err)
		}
		result.Message.AuthorUserID = userID
		result.Message.AuthorHandle = authorHandle
		if _, err := tx.Exec(ctx, `
			INSERT INTO message_revisions (message_id, revision, body, action, created_at)
			VALUES ($1, 1, $2, 'created', $3)`, result.Message.ID, body, now); err != nil {
			return fmt.Errorf("insert initial message revision: %w", err)
		}
		envelope := map[string]any{
			"event_id": eventID, "type": "message.created", "occurred_at": now,
			"community_id": result.Message.CommunityID, "room_id": roomID, "actor_id": userID,
			"schema_version": 1, "cursor": encodeCursor(result.Message),
			"payload": map[string]any{"message": result.Message},
		}
		payload, err := json.Marshal(envelope)
		if err != nil {
			return fmt.Errorf("encode message event: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO outbox_events (id, aggregate_id, community_id, room_id, event_kind, payload, created_at)
			VALUES ($1, $2, $3, $4, 'message.created', $5, $6)`,
			eventID, result.Message.ID, result.Message.CommunityID, roomID, payload, now); err != nil {
			return fmt.Errorf("insert message outbox event: %w", err)
		}
		return nil
	})
	return result, err
}

func (s *PostgresStore) UpdateMessage(ctx context.Context, userID, roomID, messageID, body string, expectedRevision int, now time.Time) (MutationResult, error) {
	return s.mutateMessage(ctx, userID, roomID, messageID, body, expectedRevision, now, false)
}

func (s *PostgresStore) DeleteMessage(ctx context.Context, userID, roomID, messageID string, expectedRevision int, now time.Time) (MutationResult, error) {
	return s.mutateMessage(ctx, userID, roomID, messageID, "", expectedRevision, now, true)
}

func (s *PostgresStore) mutateMessage(ctx context.Context, userID, roomID, messageID, body string, expectedRevision int, now time.Time, deleting bool) (MutationResult, error) {
	eventID, err := platformid.New()
	if err != nil {
		return MutationResult{}, err
	}
	result := MutationResult{EventID: eventID}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT message.id::text, message.community_id::text, message.room_id::text,
			       author_user.id::text, author_user.handle, message.client_idempotency_key,
			       message.body, message.revision, message.created_at, message.edited_at, message.deleted_at
			FROM messages message
			JOIN members author ON author.id = message.author_member_id
			JOIN users author_user ON author_user.id = author.user_id
			WHERE message.room_id = $1 AND message.id = $2
			  AND EXISTS (
			      SELECT 1 FROM members actor
			      WHERE actor.community_id = message.community_id AND actor.user_id = $3 AND actor.state = 'active'
			  )
			FOR UPDATE OF message`, roomID, messageID, userID,
		).Scan(&result.Message.ID, &result.Message.CommunityID, &result.Message.RoomID,
			&result.Message.AuthorUserID, &result.Message.AuthorHandle, &result.Message.ClientIdempotencyKey,
			&result.Message.Body, &result.Message.Revision, &result.Message.CreatedAt,
			&result.Message.EditedAt, &result.Message.DeletedAt)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && result.Message.AuthorUserID != userID) {
			return ErrNotAuthorized
		}
		if err != nil {
			return fmt.Errorf("load message for mutation: %w", err)
		}
		if result.Message.Revision != expectedRevision || result.Message.DeletedAt != nil {
			return ErrRevisionConflict
		}

		action := "updated"
		eventKind := "message.updated"
		if deleting {
			action = "deleted"
			eventKind = "message.deleted"
		}
		if deleting {
			err = tx.QueryRow(ctx, `
				UPDATE messages SET body = '', revision = revision + 1, deleted_at = $1
				WHERE id = $2
				RETURNING body, revision, edited_at, deleted_at`, now, messageID,
			).Scan(&result.Message.Body, &result.Message.Revision, &result.Message.EditedAt, &result.Message.DeletedAt)
		} else {
			err = tx.QueryRow(ctx, `
				UPDATE messages SET body = $1, revision = revision + 1, edited_at = $2
				WHERE id = $3
				RETURNING body, revision, edited_at, deleted_at`, body, now, messageID,
			).Scan(&result.Message.Body, &result.Message.Revision, &result.Message.EditedAt, &result.Message.DeletedAt)
		}
		if err != nil {
			return fmt.Errorf("mutate message: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO message_revisions (message_id, revision, body, action, created_at)
			VALUES ($1, $2, $3, $4, $5)`, messageID, result.Message.Revision, result.Message.Body, action, now); err != nil {
			return fmt.Errorf("insert message revision: %w", err)
		}
		envelope := map[string]any{
			"event_id": eventID, "type": eventKind, "occurred_at": now,
			"community_id": result.Message.CommunityID, "room_id": roomID, "actor_id": userID,
			"schema_version": 1, "cursor": encodeCursor(result.Message),
			"payload": map[string]any{"message": result.Message},
		}
		payload, err := json.Marshal(envelope)
		if err != nil {
			return fmt.Errorf("encode message mutation event: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO outbox_events (id, aggregate_id, community_id, room_id, event_kind, payload, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, eventID, messageID,
			result.Message.CommunityID, roomID, eventKind, payload, now); err != nil {
			return fmt.Errorf("insert message mutation outbox event: %w", err)
		}
		return nil
	})
	return result, err
}

func (s *PostgresStore) ListMessages(ctx context.Context, roomID string, query HistoryQuery) (HistoryResult, error) {
	const selectMessages = `
		SELECT message.id::text, message.community_id::text, message.room_id::text,
		       app_user.id::text, app_user.handle, message.client_idempotency_key,
		       message.body, message.revision, message.created_at, message.edited_at, message.deleted_at
		FROM messages message
		JOIN members author ON author.id = message.author_member_id
		JOIN users app_user ON app_user.id = author.user_id`
	var rows pgx.Rows
	var err error
	if query.Before != nil {
		rows, err = s.pool.Query(ctx, selectMessages+`
			WHERE message.room_id = $1 AND (message.created_at, message.id) < ($2, $3)
			ORDER BY message.created_at DESC, message.id DESC LIMIT $4`, roomID, query.Before.CreatedAt, query.Before.ID, query.Limit+1)
	} else if query.After != nil {
		rows, err = s.pool.Query(ctx, selectMessages+`
			WHERE message.room_id = $1 AND (message.created_at, message.id) > ($2, $3)
			ORDER BY message.created_at, message.id LIMIT $4`, roomID, query.After.CreatedAt, query.After.ID, query.Limit+1)
	} else {
		rows, err = s.pool.Query(ctx, selectMessages+`
			WHERE message.room_id = $1
			ORDER BY message.created_at DESC, message.id DESC LIMIT $2`, roomID, query.Limit+1)
	}
	if err != nil {
		return HistoryResult{}, fmt.Errorf("query message history: %w", err)
	}
	defer rows.Close()
	messages := make([]Message, 0, query.Limit+1)
	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.ID, &message.CommunityID, &message.RoomID, &message.AuthorUserID, &message.AuthorHandle, &message.ClientIdempotencyKey, &message.Body, &message.Revision, &message.CreatedAt, &message.EditedAt, &message.DeletedAt); err != nil {
			return HistoryResult{}, fmt.Errorf("scan message history: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return HistoryResult{}, fmt.Errorf("read message history: %w", err)
	}
	result := HistoryResult{Messages: messages, HasMore: len(messages) > query.Limit}
	if result.HasMore {
		result.Messages = result.Messages[:query.Limit]
	}
	if query.After == nil {
		reverseMessages(result.Messages)
	}
	return result, nil
}

func (s *PostgresStore) GetReadState(ctx context.Context, userID, roomID string) (ReadState, error) {
	var state ReadState
	var lastMessageID *string
	var lastReadAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT cursor.last_message_id::text, cursor.last_read_created_at, count(message.id)::int
		FROM rooms room
		JOIN members member ON member.community_id = room.community_id
			AND member.user_id = $1 AND member.state = 'active'
		LEFT JOIN read_cursors cursor ON cursor.member_id = member.id AND cursor.room_id = room.id
		LEFT JOIN messages message ON message.room_id = room.id AND (
			cursor.last_message_id IS NULL OR
			(message.created_at, message.id) > (cursor.last_read_created_at, cursor.last_message_id)
		)
		WHERE room.id = $2
		GROUP BY cursor.last_message_id, cursor.last_read_created_at`, userID, roomID,
	).Scan(&lastMessageID, &lastReadAt, &state.UnreadCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReadState{}, ErrNotAuthorized
	}
	if err != nil {
		return ReadState{}, fmt.Errorf("get Room read state: %w", err)
	}
	if lastMessageID != nil && lastReadAt != nil {
		state.LastReadMessageID = *lastMessageID
		state.Cursor = encodeCursor(Message{ID: *lastMessageID, CreatedAt: *lastReadAt})
	}
	return state, nil
}

func (s *PostgresStore) MarkRead(ctx context.Context, userID, roomID, messageID string, now time.Time) (ReadState, error) {
	transaction, err := s.pool.Begin(ctx)
	if err != nil {
		return ReadState{}, fmt.Errorf("begin mark Room read: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var communityID, memberID string
	var messageCreatedAt time.Time
	err = transaction.QueryRow(ctx, `
		SELECT room.community_id::text, member.id::text, message.created_at
		FROM rooms room
		JOIN members member ON member.community_id = room.community_id
			AND member.user_id = $1 AND member.state = 'active'
		JOIN messages message ON message.room_id = room.id AND message.id = $3
		WHERE room.id = $2
		FOR SHARE OF room, member, message`, userID, roomID, messageID,
	).Scan(&communityID, &memberID, &messageCreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReadState{}, ErrInvalidCursor
	}
	if err != nil {
		return ReadState{}, fmt.Errorf("resolve Room read cursor: %w", err)
	}
	if _, err := transaction.Exec(ctx, `
		INSERT INTO read_cursors (
			member_id, community_id, room_id, last_message_id, last_read_created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (member_id, room_id) DO UPDATE SET
			last_message_id = EXCLUDED.last_message_id,
			last_read_created_at = EXCLUDED.last_read_created_at,
			updated_at = EXCLUDED.updated_at
		WHERE (read_cursors.last_read_created_at, read_cursors.last_message_id) <
			(EXCLUDED.last_read_created_at, EXCLUDED.last_message_id)`,
		memberID, communityID, roomID, messageID, messageCreatedAt, now,
	); err != nil {
		return ReadState{}, fmt.Errorf("upsert Room read cursor: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return ReadState{}, fmt.Errorf("commit Room read cursor: %w", err)
	}
	return s.GetReadState(ctx, userID, roomID)
}

func reverseMessages(messages []Message) {
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
}
