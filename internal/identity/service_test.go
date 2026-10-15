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
