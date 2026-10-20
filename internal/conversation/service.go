package conversation

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SkyC0der/skyroom/internal/identity"
	"github.com/SkyC0der/skyroom/internal/permission"
	platformid "github.com/SkyC0der/skyroom/internal/platform/id"
)

type Authenticator interface {
	Authenticate(context.Context, string) (identity.AuthenticatedSession, error)
}

type Service struct {
	store       Store
	auth        Authenticator
	permissions *permission.RoomService
	broker      *Broker
	now         func() time.Time
}

func NewService(store Store, auth Authenticator) *Service {
	return &Service{store: store, auth: auth, permissions: permission.NewRoomService(store), broker: NewBroker(), now: time.Now}
}

func (s *Service) Post(ctx context.Context, sessionToken, roomID, idempotencyKey, body string) (PostResult, error) {
	authenticated, err := s.auth.Authenticate(ctx, sessionToken)
	if err != nil {
		return PostResult{}, err
	}
	if authenticated.User.Handle == nil {
		return PostResult{}, ErrNotAuthorized
	}
	if !validIdempotencyKey(idempotencyKey) || !validMessageBody(body) {
		return PostResult{}, ErrInvalidMessage
	}
	allowed, err := s.permissions.Authorize(ctx, authenticated.User.ID, roomID, permission.SendMessage)
	if err != nil {
		return PostResult{}, err
	}
	if !allowed {
		return PostResult{}, ErrNotAuthorized
	}
	result, err := s.store.CreateMessage(ctx, authenticated.User.ID, *authenticated.User.Handle, roomID, idempotencyKey, body, s.now().UTC())
	if err != nil {
		return PostResult{}, err
	}
	if !result.Duplicate {
		eventID := result.EventID
		if eventID == "" {
			eventID = result.Message.ID
		}
		s.broker.Publish(Event{
			EventID: eventID, Type: "message.created", OccurredAt: result.Message.CreatedAt,
			CommunityID: result.Message.CommunityID, RoomID: roomID, ActorID: result.Message.AuthorUserID, SchemaVersion: 1,
			Cursor: encodeCursor(result.Message), Payload: Payload{Message: result.Message},
		})
	}
	return result, nil
}

func (s *Service) Subscribe(ctx context.Context, sessionToken, roomID string) (Subscription, error) {
	authenticated, err := s.auth.Authenticate(ctx, sessionToken)
	if err != nil {
		return Subscription{}, err
	}
	allowed, err := s.permissions.Authorize(ctx, authenticated.User.ID, roomID, permission.ViewRoom)
	if err != nil {
		return Subscription{}, err
	}
	if !allowed {
		return Subscription{}, ErrNotAuthorized
	}
	return s.broker.Subscribe(roomID), nil
}

func (s *Service) ReadState(ctx context.Context, sessionToken, roomID string) (ReadState, error) {
	authenticated, err := s.authorize(ctx, sessionToken, roomID, permission.ViewRoom)
	if err != nil {
		return ReadState{}, err
	}
	return s.store.GetReadState(ctx, authenticated.User.ID, roomID)
}

func (s *Service) MarkRead(ctx context.Context, sessionToken, roomID, messageID string) (ReadState, error) {
	if !platformid.IsValid(messageID) {
		return ReadState{}, ErrInvalidCursor
	}
	authenticated, err := s.authorize(ctx, sessionToken, roomID, permission.ViewRoom)
	if err != nil {
		return ReadState{}, err
	}
	return s.store.MarkRead(ctx, authenticated.User.ID, roomID, messageID, s.now().UTC())
}

func (s *Service) authorize(ctx context.Context, sessionToken, roomID string, capability permission.Capability) (identity.AuthenticatedSession, error) {
	authenticated, err := s.auth.Authenticate(ctx, sessionToken)
	if err != nil {
		return identity.AuthenticatedSession{}, err
	}
	allowed, err := s.permissions.Authorize(ctx, authenticated.User.ID, roomID, capability)
	if err != nil {
		return identity.AuthenticatedSession{}, err
	}
	if !allowed {
		return identity.AuthenticatedSession{}, ErrNotAuthorized
	}
	return authenticated, nil
}

func (s *Service) History(ctx context.Context, sessionToken, roomID, before, after string, limit int) (HistoryPage, error) {
	authenticated, err := s.auth.Authenticate(ctx, sessionToken)
	if err != nil {
		return HistoryPage{}, err
	}
	if before != "" && after != "" {
		return HistoryPage{}, ErrInvalidCursor
	}
	query := HistoryQuery{Limit: limit}
	if query.Limit == 0 {
		query.Limit = defaultHistoryLimit
	}
	if query.Limit < 1 || query.Limit > maxHistoryLimit {
		return HistoryPage{}, ErrInvalidCursor
	}
	if before != "" {
		cursor, err := decodeCursor(before)
		if err != nil {
			return HistoryPage{}, ErrInvalidCursor
		}
		query.Before = &cursor
	}
	if after != "" {
		cursor, err := decodeCursor(after)
		if err != nil {
			return HistoryPage{}, ErrInvalidCursor
		}
		query.After = &cursor
	}
	allowed, err := s.permissions.Authorize(ctx, authenticated.User.ID, roomID, permission.ViewRoom)
	if err != nil {
		return HistoryPage{}, err
	}
	if !allowed {
		return HistoryPage{}, ErrNotAuthorized
	}
	result, err := s.store.ListMessages(ctx, roomID, query)
	if err != nil {
		return HistoryPage{}, err
	}
	page := HistoryPage{Messages: result.Messages}
	if len(result.Messages) > 0 {
		page.OldestCursor = encodeCursor(result.Messages[0])
		page.LatestCursor = encodeCursor(result.Messages[len(result.Messages)-1])
	}
	if query.After != nil {
		page.HasMoreAfter = result.HasMore
	} else {
		page.HasMoreBefore = result.HasMore
	}
	return page, nil
}

func validMessageBody(body string) bool {
	length := utf8.RuneCountInString(body)
	return length >= 1 && length <= 4000 && strings.TrimSpace(body) != "" && !strings.ContainsRune(body, 0)
}

func validIdempotencyKey(value string) bool {
	if len(value) < 8 || len(value) > 100 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func encodeCursor(message Message) string {
	value := strconv.FormatInt(message.CreatedAt.UTC().UnixNano(), 10) + ":" + message.ID
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeCursor(value string) (Cursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return Cursor{}, err
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 || !platformid.IsValid(parts[1]) {
		return Cursor{}, fmt.Errorf("invalid cursor")
	}
	nanoseconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return Cursor{}, err
	}
	return Cursor{CreatedAt: time.Unix(0, nanoseconds).UTC(), ID: parts[1]}, nil
}
