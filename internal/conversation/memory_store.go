package conversation

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/SkyC0der/skyroom/internal/permission"
	platformid "github.com/SkyC0der/skyroom/internal/platform/id"
)

type memoryRoomStore interface {
	permission.RoomStore
	CommunityForRoom(ctx context.Context, roomID string) (string, bool)
}

type MemoryStore struct {
	mu          sync.Mutex
	rooms       memoryRoomStore
	messages    map[string][]Message
	idempotency map[string]Message
}

func NewMemoryStore(rooms memoryRoomStore) *MemoryStore {
	return &MemoryStore{rooms: rooms, messages: make(map[string][]Message), idempotency: make(map[string]Message)}
}

func (s *MemoryStore) HasRoomCapability(ctx context.Context, userID, roomID string, capability permission.Capability) (bool, error) {
	return s.rooms.HasRoomCapability(ctx, userID, roomID, capability)
}

func (s *MemoryStore) CreateMessage(ctx context.Context, userID, authorHandle, roomID, idempotencyKey, body string, now time.Time) (PostResult, error) {
	communityID, ok := s.rooms.CommunityForRoom(ctx, roomID)
	if !ok {
		return PostResult{}, ErrNotAuthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := userID + ":" + idempotencyKey
	if existing, ok := s.idempotency[key]; ok {
		if existing.RoomID != roomID || existing.Body != body {
			return PostResult{}, ErrIdempotencyConflict
		}
		return PostResult{Message: existing, Duplicate: true}, nil
	}
	id, err := platformid.New()
	if err != nil {
		return PostResult{}, err
	}
	message := Message{
		ID: id, CommunityID: communityID, RoomID: roomID, AuthorUserID: userID, AuthorHandle: authorHandle,
		ClientIdempotencyKey: idempotencyKey, Body: body, Revision: 1, CreatedAt: now,
	}
	s.messages[roomID] = append(s.messages[roomID], message)
	s.idempotency[key] = message
	return PostResult{Message: message}, nil
}

func (s *MemoryStore) ListMessages(_ context.Context, roomID string, query HistoryQuery) (HistoryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := append(make([]Message, 0, len(s.messages[roomID])), s.messages[roomID]...)
	sort.Slice(messages, func(i, j int) bool { return messageLess(messages[i], messages[j]) })
	filtered := make([]Message, 0, len(messages))
	for _, message := range messages {
		if query.Before != nil && !messageBeforeCursor(message, *query.Before) {
			continue
		}
		if query.After != nil && !messageAfterCursor(message, *query.After) {
			continue
		}
		filtered = append(filtered, message)
	}
	result := HistoryResult{Messages: filtered}
	if len(filtered) <= query.Limit {
		return result, nil
	}
	result.HasMore = true
	if query.After != nil {
		result.Messages = filtered[:query.Limit]
	} else {
		result.Messages = filtered[len(filtered)-query.Limit:]
	}
	return result, nil
}

func messageLess(left, right Message) bool {
	if left.CreatedAt.Equal(right.CreatedAt) {
		return left.ID < right.ID
	}
	return left.CreatedAt.Before(right.CreatedAt)
}

func messageBeforeCursor(message Message, cursor Cursor) bool {
	return message.CreatedAt.Before(cursor.CreatedAt) || (message.CreatedAt.Equal(cursor.CreatedAt) && message.ID < cursor.ID)
}

func messageAfterCursor(message Message, cursor Cursor) bool {
	return message.CreatedAt.After(cursor.CreatedAt) || (message.CreatedAt.Equal(cursor.CreatedAt) && message.ID > cursor.ID)
}
