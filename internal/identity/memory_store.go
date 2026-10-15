package identity

import (
	"context"
	"crypto/hmac"
	"sync"
	"time"
)

// MemoryStore supports tests and the earliest local scaffold. Production uses
// the same Store contract with PostgreSQL persistence.
type MemoryStore struct {
	mu         sync.Mutex
	challenges map[string][]Challenge
	users      map[string]User
	sessions   map[string]Session
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		challenges: make(map[string][]Challenge),
		users:      make(map[string]User),
		sessions:   make(map[string]Session),
	}
}

func (s *MemoryStore) CreateChallenge(_ context.Context, challenge Challenge, minimumInterval time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing := s.challenges[challenge.EmailNormalized]
	if len(existing) > 0 {
		latest := existing[len(existing)-1]
		if challenge.CreatedAt.Sub(latest.CreatedAt) < minimumInterval {
			return ErrResendTooSoon
		}
	}
	s.challenges[challenge.EmailNormalized] = append(existing, challenge)
	return nil
}

func (s *MemoryStore) DeleteChallenge(_ context.Context, challengeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for email, challenges := range s.challenges {
		for index, challenge := range challenges {
			if challenge.ID == challengeID {
				s.challenges[email] = append(challenges[:index], challenges[index+1:]...)
				return nil
			}
		}
	}
	return nil
}

func (s *MemoryStore) CompleteVerification(_ context.Context, email string, codeHash []byte, now time.Time, session Session) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	challenges := s.challenges[email]
	if len(challenges) == 0 {
		return User{}, ErrInvalidCode
	}
	challenge := &challenges[len(challenges)-1]
	if challenge.ConsumedAt != nil {
		return User{}, ErrChallengeConsumed
	}
	if !now.Before(challenge.ExpiresAt) {
		return User{}, ErrChallengeExpired
	}
	if challenge.AttemptsUsed >= challenge.MaxAttempts {
		return User{}, ErrTooManyAttempts
	}
	if !hmac.Equal(challenge.CodeHash, codeHash) {
		challenge.AttemptsUsed++
		if challenge.AttemptsUsed >= challenge.MaxAttempts {
			return User{}, ErrTooManyAttempts
		}
		return User{}, ErrInvalidCode
	}

	user, exists := s.users[email]
	if !exists {
		userID, err := newID()
		if err != nil {
			return User{}, err
		}
		user = User{
			ID: userID, Email: email, EmailNormalized: email,
			EmailVerifiedAt: now, CreatedAt: now,
		}
		s.users[email] = user
	}
	session.UserID = user.ID
	s.sessions[session.ID] = session
	challenge.ConsumedAt = &now
	return user, nil
}
