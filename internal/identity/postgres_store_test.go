package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/SkyC0der/skyroom/internal/platform/postgres/pgtest"
)

func TestPostgresSessionAndHandleFlow(t *testing.T) {
	t.Parallel()

	pool := pgtest.MigratedPool(t)
	sender := &capturedSender{}
	service := NewService(NewPostgresStore(pool), sender, "test-secret")

	verify := func(email string) string {
		t.Helper()
		if err := service.RequestCode(context.Background(), email); err != nil {
			t.Fatal(err)
		}
		verified, err := service.VerifyCode(context.Background(), email, sender.code)
		if err != nil {
			t.Fatal(err)
		}
		return verified.Token
	}

	firstToken := verify("first@example.com")
	authenticated, err := service.ClaimHandle(context.Background(), firstToken, "First.Member")
	if err != nil {
		t.Fatal(err)
	}
	if authenticated.User.Handle == nil || *authenticated.User.Handle != "first.member" {
		t.Fatalf("unexpected handle: %+v", authenticated.User.Handle)
	}

	secondToken := verify("second@example.com")
	if _, err := service.ClaimHandle(context.Background(), secondToken, "FIRST.MEMBER"); !errors.Is(err, ErrHandleTaken) {
		t.Fatalf("expected unique handle conflict, got %v", err)
	}

	if err := service.SignOut(context.Background(), firstToken); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), firstToken); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected revoked PostgreSQL session to fail, got %v", err)
	}
}
