package identity

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidEmail      = errors.New("invalid email")
	ErrInvalidCode       = errors.New("invalid verification code")
	ErrChallengeExpired  = errors.New("verification challenge expired")
	ErrChallengeConsumed = errors.New("verification challenge consumed")
	ErrTooManyAttempts   = errors.New("too many verification attempts")
	ErrResendTooSoon     = errors.New("verification code requested too recently")
)

type Challenge struct {
	ID              string
	EmailNormalized string
	CodeHash        []byte
	AttemptsUsed    int
	MaxAttempts     int
	ExpiresAt       time.Time
	ConsumedAt      *time.Time
	CreatedAt       time.Time
}

type User struct {
	ID              string    `json:"id"`
	Email           string    `json:"email"`
	EmailNormalized string    `json:"-"`
	Handle          *string   `json:"handle"`
	EmailVerifiedAt time.Time `json:"email_verified_at"`
	CreatedAt       time.Time `json:"created_at"`
}

type Session struct {
	ID         string
	UserID     string
	TokenHash  []byte
	ExpiresAt  time.Time
	LastSeenAt time.Time
	CreatedAt  time.Time
}

type Store interface {
	CreateChallenge(context.Context, Challenge, time.Duration) error
	DeleteChallenge(context.Context, string) error
	VerifyChallenge(context.Context, string, []byte, time.Time) error
	FindOrCreateUser(context.Context, string, time.Time) (User, error)
	CreateSession(context.Context, Session) error
}

type CodeSender interface {
	SendVerificationCode(context.Context, string, string) error
}
