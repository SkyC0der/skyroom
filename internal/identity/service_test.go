package identity

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type capturedSender struct {
	mu    sync.Mutex
	code  string
	email string
}

func (s *capturedSender) SendVerificationCode(_ context.Context, email, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.email, s.code = email, code
	return nil
}

func TestRequestAndVerifyCodeCreatesSession(t *testing.T) {
	t.Parallel()

	sender := &capturedSender{}
	service := NewService(NewMemoryStore(), sender, "test-secret")
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	if err := service.RequestCode(context.Background(), "Sky@AIESEC.net"); err != nil {
		t.Fatalf("request code: %v", err)
	}
	if sender.email != "sky@aiesec.net" {
		t.Fatalf("expected normalized email, got %q", sender.email)
	}

	verified, err := service.VerifyCode(context.Background(), "sky@aiesec.net", sender.code)
	if err != nil {
		t.Fatalf("verify code: %v", err)
	}
	if verified.User.Email != "sky@aiesec.net" || verified.Token == "" {
		t.Fatalf("unexpected verified session: %+v", verified)
	}
	if want := now.Add(30 * 24 * time.Hour); !verified.ExpiresAt.Equal(want) {
		t.Fatalf("expected expiry %v, got %v", want, verified.ExpiresAt)
	}
}

func TestCodeCannotBeReused(t *testing.T) {
	t.Parallel()

	sender := &capturedSender{}
	service := NewService(NewMemoryStore(), sender, "test-secret")
	if err := service.RequestCode(context.Background(), "member@example.com"); err != nil {
		t.Fatalf("request code: %v", err)
	}
	if _, err := service.VerifyCode(context.Background(), "member@example.com", sender.code); err != nil {
		t.Fatalf("first verification: %v", err)
	}
	if _, err := service.VerifyCode(context.Background(), "member@example.com", sender.code); !errors.Is(err, ErrChallengeConsumed) {
		t.Fatalf("expected consumed challenge error, got %v", err)
	}
}

func TestCodeExpiresAndResendIsThrottled(t *testing.T) {
	t.Parallel()

	sender := &capturedSender{}
	service := NewService(NewMemoryStore(), sender, "test-secret")
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	if err := service.RequestCode(context.Background(), "member@example.com"); err != nil {
		t.Fatalf("request code: %v", err)
	}
	if err := service.RequestCode(context.Background(), "member@example.com"); !errors.Is(err, ErrResendTooSoon) {
		t.Fatalf("expected resend throttle, got %v", err)
	}
	now = now.Add(11 * time.Minute)
	if _, err := service.VerifyCode(context.Background(), "member@example.com", sender.code); !errors.Is(err, ErrChallengeExpired) {
		t.Fatalf("expected expired challenge, got %v", err)
	}
}

func TestInvalidEmailIsRejected(t *testing.T) {
	t.Parallel()
	service := NewService(NewMemoryStore(), &capturedSender{}, "test-secret")
	if err := service.RequestCode(context.Background(), "not-an-email"); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("expected invalid email, got %v", err)
	}
}

func TestSessionCanAuthenticateClaimHandleAndSignOut(t *testing.T) {
	t.Parallel()

	sender := &capturedSender{}
	service := NewService(NewMemoryStore(), sender, "test-secret")
	if err := service.RequestCode(context.Background(), "member@example.com"); err != nil {
		t.Fatal(err)
	}
	verified, err := service.VerifyCode(context.Background(), "member@example.com", sender.code)
	if err != nil {
		t.Fatal(err)
	}

	authenticated, err := service.Authenticate(context.Background(), verified.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if authenticated.User.Email != "member@example.com" {
		t.Fatalf("unexpected authenticated user: %+v", authenticated.User)
	}
	if authenticated.ExpiresAt.Sub(time.Now().UTC()) < sessionLifetime-time.Minute {
		t.Fatalf("expected rolling session expiry, got %v", authenticated.ExpiresAt)
	}

	authenticated, err = service.ClaimHandle(context.Background(), verified.Token, "Sky.Room")
	if err != nil {
		t.Fatalf("claim handle: %v", err)
	}
	if authenticated.User.Handle == nil || *authenticated.User.Handle != "sky.room" {
		t.Fatalf("expected canonical handle, got %+v", authenticated.User.Handle)
	}

	if err := service.SignOut(context.Background(), verified.Token); err != nil {
		t.Fatalf("sign out: %v", err)
	}
	if _, err := service.Authenticate(context.Background(), verified.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected revoked session to be rejected, got %v", err)
	}
}

func TestHandleValidationUniquenessAndFreshVerification(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	sender := &capturedSender{}
	service := NewService(store, sender, "test-secret")
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

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
	if _, err := service.ClaimHandle(context.Background(), firstToken, "bad handle"); !errors.Is(err, ErrInvalidHandle) {
		t.Fatalf("expected invalid handle, got %v", err)
	}
	if _, err := service.ClaimHandle(context.Background(), firstToken, "member_one"); err != nil {
		t.Fatal(err)
	}
	secondToken := verify("second@example.com")
	if _, err := service.ClaimHandle(context.Background(), secondToken, "MEMBER_ONE"); !errors.Is(err, ErrHandleTaken) {
		t.Fatalf("expected handle conflict, got %v", err)
	}

	now = now.Add(freshVerificationLifetime + time.Second)
	if _, err := service.ClaimHandle(context.Background(), secondToken, "member_two"); !errors.Is(err, ErrVerificationStale) {
		t.Fatalf("expected fresh verification requirement, got %v", err)
	}
}
