package conversation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SkyC0der/skyroom/internal/community"
	"github.com/SkyC0der/skyroom/internal/identity"
)

type testSender struct {
	code string
}

func (s *testSender) SendVerificationCode(_ context.Context, _, code string) error {
	s.code = code
	return nil
}

func verifiedUser(t *testing.T, auth *identity.Service, sender *testSender, email, handle string) string {
	t.Helper()
	ctx := context.Background()
	if err := auth.RequestCode(ctx, email); err != nil {
		t.Fatal(err)
	}
	verified, err := auth.VerifyCode(ctx, email, sender.code)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.ClaimHandle(ctx, verified.Token, handle); err != nil {
		t.Fatal(err)
	}
	return verified.Token
}

func TestPostIsAuthorizedIdempotentAndPaginated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sender := &testSender{}
	auth := identity.NewService(identity.NewMemoryStore(), sender, "test-secret")
	communities := community.NewMemoryStore()
	communityService := community.NewService(communities, auth)
	service := NewService(NewMemoryStore(communities), auth)
	ownerToken := verifiedUser(t, auth, sender, "owner@example.com", "message.owner")
	created, err := communityService.Create(ctx, ownerToken, "Message Guild", "")
	if err != nil {
		t.Fatal(err)
	}

	started := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return started }
	first, err := service.Post(ctx, ownerToken, created.GeneralRoom.ID, "client-key-1", "First message")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := service.Post(ctx, ownerToken, created.GeneralRoom.ID, "client-key-1", "First message")
	if err != nil || !repeated.Duplicate || repeated.Message.ID != first.Message.ID {
		t.Fatalf("expected idempotent result, result=%+v err=%v", repeated, err)
	}
	if _, err := service.Post(ctx, ownerToken, created.GeneralRoom.ID, "client-key-1", "Changed body"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	for index, item := range []struct{ key, body string }{{"client-key-2", "Second message"}, {"client-key-3", "Third message"}} {
		messageTime := started.Add(time.Duration(index+1) * time.Second)
		service.now = func() time.Time { return messageTime }
		if _, err := service.Post(ctx, ownerToken, created.GeneralRoom.ID, item.key, item.body); err != nil {
			t.Fatal(err)
		}
	}
	page, err := service.History(ctx, ownerToken, created.GeneralRoom.ID, "", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 || page.Messages[0].Body != "Second message" || page.Messages[1].Body != "Third message" || !page.HasMoreBefore {
		t.Fatalf("unexpected latest page: %+v", page)
	}
	older, err := service.History(ctx, ownerToken, created.GeneralRoom.ID, page.OldestCursor, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Messages) != 1 || older.Messages[0].Body != "First message" || older.HasMoreBefore {
		t.Fatalf("unexpected older page: %+v", older)
	}

	outsiderToken := verifiedUser(t, auth, sender, "outsider@example.com", "message.outsider")
	if _, err := service.Post(ctx, outsiderToken, created.GeneralRoom.ID, "outsider-key", "Not allowed"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("expected authorization failure, got %v", err)
	}
}

func TestHistoryRejectsInvalidCursor(t *testing.T) {
	t.Parallel()
	sender := &testSender{}
	auth := identity.NewService(identity.NewMemoryStore(), sender, "test-secret")
	communities := community.NewMemoryStore()
	communityService := community.NewService(communities, auth)
	service := NewService(NewMemoryStore(communities), auth)
	token := verifiedUser(t, auth, sender, "cursor@example.com", "cursor.owner")
	created, err := communityService.Create(context.Background(), token, "Cursor Guild", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.History(context.Background(), token, created.GeneralRoom.ID, "not-a-cursor", "", 20); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("expected invalid cursor, got %v", err)
	}
}

func TestSubscribeAuthorizesAndPublishesCommittedMessagesOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sender := &testSender{}
	auth := identity.NewService(identity.NewMemoryStore(), sender, "test-secret")
	communities := community.NewMemoryStore()
	communityService := community.NewService(communities, auth)
	service := NewService(NewMemoryStore(communities), auth)
	ownerToken := verifiedUser(t, auth, sender, "live-owner@example.com", "live.owner")
	created, err := communityService.Create(ctx, ownerToken, "Live Guild", "")
	if err != nil {
		t.Fatal(err)
	}

	subscription, err := service.Subscribe(ctx, ownerToken, created.GeneralRoom.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Cancel()
	posted, err := service.Post(ctx, ownerToken, created.GeneralRoom.ID, "live-client-key", "Arrives live")
	if err != nil {
		t.Fatal(err)
	}
	event := <-subscription.Events
	if event.Type != "message.created" || event.EventID != posted.Message.ID || event.Cursor == "" || event.Payload.Message.Body != "Arrives live" {
		t.Fatalf("unexpected event: %+v", event)
	}
	if _, err := service.Post(ctx, ownerToken, created.GeneralRoom.ID, "live-client-key", "Arrives live"); err != nil {
		t.Fatal(err)
	}
	select {
	case duplicate := <-subscription.Events:
		t.Fatalf("idempotent retry published a duplicate event: %+v", duplicate)
	default:
	}

	outsiderToken := verifiedUser(t, auth, sender, "live-outsider@example.com", "live.outsider")
	if _, err := service.Subscribe(ctx, outsiderToken, created.GeneralRoom.ID); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("expected subscription authorization failure, got %v", err)
	}
}

func TestReadStateCountsUnreadAndNeverMovesBackward(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sender := &testSender{}
	auth := identity.NewService(identity.NewMemoryStore(), sender, "test-secret")
	communities := community.NewMemoryStore()
	communityService := community.NewService(communities, auth)
	service := NewService(NewMemoryStore(communities), auth)
	token := verifiedUser(t, auth, sender, "reader@example.com", "room.reader")
	created, err := communityService.Create(ctx, token, "Readers", "")
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC) }
	first, err := service.Post(ctx, token, created.GeneralRoom.ID, "read-client-1", "First unread")
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 10, 20, 10, 0, 1, 0, time.UTC) }
	second, err := service.Post(ctx, token, created.GeneralRoom.ID, "read-client-2", "Second unread")
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.ReadState(ctx, token, created.GeneralRoom.ID)
	if err != nil || state.UnreadCount != 2 || state.LastReadMessageID != "" {
		t.Fatalf("unexpected initial read state: state=%+v err=%v", state, err)
	}
	state, err = service.MarkRead(ctx, token, created.GeneralRoom.ID, second.Message.ID)
	if err != nil || state.UnreadCount != 0 || state.LastReadMessageID != second.Message.ID || state.Cursor == "" {
		t.Fatalf("unexpected marked read state: state=%+v err=%v", state, err)
	}
	state, err = service.MarkRead(ctx, token, created.GeneralRoom.ID, first.Message.ID)
	if err != nil || state.LastReadMessageID != second.Message.ID {
		t.Fatalf("read cursor moved backward: state=%+v err=%v", state, err)
	}
	if _, err := service.MarkRead(ctx, token, created.GeneralRoom.ID, "not-a-message"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("expected invalid read cursor, got %v", err)
	}
}
