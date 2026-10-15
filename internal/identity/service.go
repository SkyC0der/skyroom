package identity

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/mail"
	"strings"
	"time"
)

const (
	codeLifetime    = 10 * time.Minute
	resendInterval  = 60 * time.Second
	sessionLifetime = 30 * 24 * time.Hour
	maxAttempts     = 5
)

type clock func() time.Time

type Service struct {
	store  Store
	sender CodeSender
	secret []byte
	now    clock
}

type VerifiedSession struct {
	User      User
	Token     string
	ExpiresAt time.Time
}

func NewService(store Store, sender CodeSender, secret string) *Service {
	return &Service{store: store, sender: sender, secret: []byte(secret), now: time.Now}
}

func (s *Service) RequestCode(ctx context.Context, email string) error {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return err
	}

	code, err := generateCode()
	if err != nil {
		return fmt.Errorf("generate verification code: %w", err)
	}
	now := s.now().UTC()
	challengeID, err := newID()
	if err != nil {
		return fmt.Errorf("generate challenge ID: %w", err)
	}
	challenge := Challenge{
		ID:              challengeID,
		EmailNormalized: normalized,
		CodeHash:        s.hashCode(normalized, code),
		MaxAttempts:     maxAttempts,
		ExpiresAt:       now.Add(codeLifetime),
		CreatedAt:       now,
	}
	if err := s.store.CreateChallenge(ctx, challenge, resendInterval); err != nil {
		return err
	}
	if err := s.sender.SendVerificationCode(ctx, normalized, code); err != nil {
		_ = s.store.DeleteChallenge(ctx, challenge.ID)
		return fmt.Errorf("send verification code: %w", err)
	}
	return nil
}

func (s *Service) VerifyCode(ctx context.Context, email, code string) (VerifiedSession, error) {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return VerifiedSession{}, err
	}
	if len(code) != 6 {
		return VerifiedSession{}, ErrInvalidCode
	}
	for _, character := range code {
		if character < '0' || character > '9' {
			return VerifiedSession{}, ErrInvalidCode
		}
	}

	now := s.now().UTC()
	rawToken, tokenHash, err := generateSessionToken()
	if err != nil {
		return VerifiedSession{}, fmt.Errorf("generate session token: %w", err)
	}
	sessionID, err := newID()
	if err != nil {
		return VerifiedSession{}, fmt.Errorf("generate session ID: %w", err)
	}
	expiresAt := now.Add(sessionLifetime)
	user, err := s.store.CompleteVerification(ctx, normalized, s.hashCode(normalized, code), now, Session{
		ID: sessionID, TokenHash: tokenHash,
		ExpiresAt: expiresAt, LastSeenAt: now, CreatedAt: now,
	})
	if err != nil {
		return VerifiedSession{}, err
	}
	return VerifiedSession{User: user, Token: rawToken, ExpiresAt: expiresAt}, nil
}

func (s *Service) hashCode(email, code string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(email))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(code))
	return mac.Sum(nil)
}

func normalizeEmail(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 254 {
		return "", ErrInvalidEmail
	}
	address, err := mail.ParseAddress(value)
	if err != nil || !strings.Contains(address.Address, "@") || address.Address != value {
		return "", ErrInvalidEmail
	}
	parts := strings.Split(address.Address, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", ErrInvalidEmail
	}
	return strings.ToLower(address.Address), nil
}

func generateCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}

func generateSessionToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	return token, hash[:], nil
}

func newID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}
