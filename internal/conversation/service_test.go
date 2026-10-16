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
