package conversation

import (
	"context"
	"errors"
	"time"

	"github.com/SkyC0der/skyroom/internal/permission"
)

var (
	ErrInvalidMessage      = errors.New("invalid message")
	ErrNotAuthorized       = errors.New("not authorized")
	ErrIdempotencyConflict = errors.New("idempotency key reused with different content")
	ErrInvalidCursor       = errors.New("invalid conversation cursor")
)

const (
	defaultHistoryLimit = 50
	maxHistoryLimit     = 100
)

type Message struct {
	ID                   string    `json:"id"`
	CommunityID          string    `json:"community_id"`
	RoomID               string    `json:"room_id"`
	AuthorUserID         string    `json:"author_user_id"`
	AuthorHandle         string    `json:"author_handle"`
	ClientIdempotencyKey string    `json:"client_idempotency_key"`
	Body                 string    `json:"body"`
	Revision             int       `json:"revision"`
	CreatedAt            time.Time `json:"created_at"`
}

type PostResult struct {
	Message   Message `json:"message"`
	Duplicate bool    `json:"duplicate"`
	EventID   string  `json:"-"`
}

type HistoryPage struct {
	Messages      []Message `json:"messages"`
	OldestCursor  string    `json:"oldest_cursor,omitempty"`
	LatestCursor  string    `json:"latest_cursor,omitempty"`
	HasMoreBefore bool      `json:"has_more_before"`
	HasMoreAfter  bool      `json:"has_more_after"`
}

type Cursor struct {
	CreatedAt time.Time
	ID        string
}

type HistoryQuery struct {
	Before *Cursor
	After  *Cursor
	Limit  int
}

type HistoryResult struct {
	Messages []Message
	HasMore  bool
}

type ReadState struct {
	LastReadMessageID string `json:"last_read_message_id,omitempty"`
	Cursor            string `json:"cursor,omitempty"`
	UnreadCount       int    `json:"unread_count"`
}

type Store interface {
	permission.RoomStore
	CreateMessage(ctx context.Context, userID, authorHandle, roomID, idempotencyKey, body string, now time.Time) (PostResult, error)
	ListMessages(ctx context.Context, roomID string, query HistoryQuery) (HistoryResult, error)
	GetReadState(ctx context.Context, userID, roomID string) (ReadState, error)
	MarkRead(ctx context.Context, userID, roomID, messageID string, now time.Time) (ReadState, error)
}
