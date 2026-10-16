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

	platformid "github.com/SkyC0der/skyroom/internal/platform/id"
)

const (
	codeLifetime              = 10 * time.Minute
	resendInterval            = 60 * time.Second
	sessionLifetime           = 30 * 24 * time.Hour
	freshVerificationLifetime = 15 * time.Minute
	maxAttempts               = 5
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

func (s *Service) Authenticate(ctx context.Context, token string) (AuthenticatedSession, error) {
	tokenHash, err := hashSessionToken(token)
	if err != nil {
		return AuthenticatedSession{}, ErrUnauthenticated
	}
	now := s.now().UTC()
	expiresAt := now.Add(sessionLifetime)
	user, session, err := s.store.AuthenticateSession(ctx, tokenHash, now, expiresAt)
	if err != nil {
		return AuthenticatedSession{}, err
	}
	return AuthenticatedSession{
		User: user, SessionID: session.ID, ExpiresAt: session.ExpiresAt,
		VerifiedAt: session.CreatedAt,
	}, nil
}

func (s *Service) SignOut(ctx context.Context, token string) error {
	tokenHash, err := hashSessionToken(token)
	if err != nil {
		return nil
	}
	return s.store.RevokeSession(ctx, tokenHash, s.now().UTC())
}

func (s *Service) ClaimHandle(ctx context.Context, token, handle string) (AuthenticatedSession, error) {
	authenticated, err := s.Authenticate(ctx, token)
	if err != nil {
		return AuthenticatedSession{}, err
	}
	if s.now().UTC().Sub(authenticated.VerifiedAt) > freshVerificationLifetime {
		return AuthenticatedSession{}, ErrVerificationStale
	}
	canonical, err := normalizeHandle(handle)
	if err != nil {
		return AuthenticatedSession{}, err
	}
	user, err := s.store.ClaimHandle(ctx, authenticated.User.ID, canonical, canonical, s.now().UTC())
	if err != nil {
		return AuthenticatedSession{}, err
	}
	authenticated.User = user
	return authenticated, nil
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

func hashSessionToken(token string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, ErrUnauthenticated
	}
	hash := sha256.Sum256([]byte(token))
	return hash[:], nil
}

func normalizeHandle(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) < 3 || len(value) > 32 {
		return "", ErrInvalidHandle
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			continue
		}
		if (character == '_' || character == '.') && index > 0 && index < len(value)-1 {
			continue
		}
		return "", ErrInvalidHandle
	}
	return value, nil
}

func newID() (string, error) {
	return platformid.New()
}
