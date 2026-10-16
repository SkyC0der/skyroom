package identity

import (
	"context"
	"crypto/hmac"
	"strings"
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

func (s *MemoryStore) AuthenticateSession(_ context.Context, tokenHash []byte, now, expiresAt time.Time) (User, Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, session := range s.sessions {
		if !hmac.Equal(session.TokenHash, tokenHash) || !now.Before(session.ExpiresAt) {
			continue
		}
		for _, user := range s.users {
			if user.ID == session.UserID {
				session.LastSeenAt = now
				session.ExpiresAt = expiresAt
				s.sessions[id] = session
				return user, session, nil
			}
		}
	}
	return User{}, Session{}, ErrUnauthenticated
}

func (s *MemoryStore) RevokeSession(_ context.Context, tokenHash []byte, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, session := range s.sessions {
		if hmac.Equal(session.TokenHash, tokenHash) {
			delete(s.sessions, id)
			break
		}
	}
	return nil
}

func (s *MemoryStore) ClaimHandle(_ context.Context, userID, handle, normalized string, _ time.Time) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, user := range s.users {
		if user.Handle != nil && strings.EqualFold(*user.Handle, normalized) && user.ID != userID {
			return User{}, ErrHandleTaken
		}
	}
	for email, user := range s.users {
		if user.ID == userID {
			user.Handle = &handle
			s.users[email] = user
			return user, nil
		}
	}
	return User{}, ErrUnauthenticated
}
