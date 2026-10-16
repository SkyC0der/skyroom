package identity

import (
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is the durable Store. Each operation runs in one transaction;
// row locks and advisory locks keep the attempt limit, single consumption, and
// resend interval correct when requests for the same email race.
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) CreateChallenge(ctx context.Context, challenge Challenge, minimumInterval time.Duration) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Serialize challenge creation per email so two concurrent requests
		// cannot both pass the resend check.
		if _, err := tx.Exec(ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended('identity.challenge:' || $1, 0))`,
			challenge.EmailNormalized,
		); err != nil {
			return fmt.Errorf("lock challenge email: %w", err)
		}

		var latest *time.Time
		if err := tx.QueryRow(ctx,
			`SELECT max(created_at) FROM email_verification_challenges WHERE email_normalized = $1`,
			challenge.EmailNormalized,
		).Scan(&latest); err != nil {
			return fmt.Errorf("read latest challenge: %w", err)
		}
		if latest != nil && challenge.CreatedAt.Sub(*latest) < minimumInterval {
			return ErrResendTooSoon
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO email_verification_challenges
				(id, email_normalized, code_hash, attempts_used, max_attempts, expires_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			challenge.ID, challenge.EmailNormalized, challenge.CodeHash,
			challenge.AttemptsUsed, challenge.MaxAttempts, challenge.ExpiresAt, challenge.CreatedAt,
		); err != nil {
			return fmt.Errorf("insert challenge: %w", err)
		}
		return nil
	})
}

func (s *PostgresStore) DeleteChallenge(ctx context.Context, challengeID string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM email_verification_challenges WHERE id = $1`, challengeID); err != nil {
		return fmt.Errorf("delete challenge: %w", err)
	}
	return nil
}

func (s *PostgresStore) CompleteVerification(ctx context.Context, email string, codeHash []byte, now time.Time, session Session) (User, error) {
	var user User
	// A wrong code must commit its attempt increment, so the domain error is
	// carried out of the transaction instead of rolling it back.
	var outcome error

	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var (
			challengeID  string
			storedHash   []byte
			attemptsUsed int
			maxAttempts  int
			expiresAt    time.Time
			consumedAt   *time.Time
		)
		err := tx.QueryRow(ctx, `
			SELECT id::text, code_hash, attempts_used, max_attempts, expires_at, consumed_at
			FROM email_verification_challenges
			WHERE email_normalized = $1
			ORDER BY created_at DESC
			LIMIT 1
			FOR UPDATE`,
			email,
		).Scan(&challengeID, &storedHash, &attemptsUsed, &maxAttempts, &expiresAt, &consumedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			outcome = ErrInvalidCode
			return nil
		}
		if err != nil {
			return fmt.Errorf("lock latest challenge: %w", err)
		}

		switch {
		case consumedAt != nil:
			outcome = ErrChallengeConsumed
			return nil
		case !now.Before(expiresAt):
			outcome = ErrChallengeExpired
			return nil
		case attemptsUsed >= maxAttempts:
			outcome = ErrTooManyAttempts
			return nil
		}

		if !hmac.Equal(storedHash, codeHash) {
			attemptsUsed++
			if _, err := tx.Exec(ctx,
				`UPDATE email_verification_challenges SET attempts_used = $2 WHERE id = $1`,
				challengeID, attemptsUsed,
			); err != nil {
				return fmt.Errorf("record failed attempt: %w", err)
			}
			outcome = ErrInvalidCode
			if attemptsUsed >= maxAttempts {
				outcome = ErrTooManyAttempts
			}
			return nil
		}

		if _, err := tx.Exec(ctx,
			`UPDATE email_verification_challenges SET consumed_at = $2 WHERE id = $1`,
			challengeID, now,
		); err != nil {
			return fmt.Errorf("consume challenge: %w", err)
		}

		user, err = findOrCreateUser(ctx, tx, email, now)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO sessions (id, user_id, token_hash, expires_at, last_seen_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			session.ID, user.ID, session.TokenHash, session.ExpiresAt, session.LastSeenAt, session.CreatedAt,
		); err != nil {
			return fmt.Errorf("insert session: %w", err)
		}
		return nil
	})
	if err != nil {
		return User{}, err
	}
	if outcome != nil {
		return User{}, outcome
	}
	return user, nil
}

func findOrCreateUser(ctx context.Context, tx pgx.Tx, email string, now time.Time) (User, error) {
	const columns = `id::text, email, email_normalized, handle, email_verified_at, created_at`
	scan := func(row pgx.Row) (User, error) {
		var user User
		err := row.Scan(&user.ID, &user.Email, &user.EmailNormalized, &user.Handle, &user.EmailVerifiedAt, &user.CreatedAt)
		return user, err
	}

	user, err := scan(tx.QueryRow(ctx, `
		INSERT INTO users (email, email_normalized, email_verified_at, created_at, updated_at)
		VALUES ($1, $1, $2, $2, $2)
		ON CONFLICT (email_normalized) DO NOTHING
		RETURNING `+columns,
		email, now,
	))
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return User{}, fmt.Errorf("insert user: %w", err)
	}

	// The User already existed, possibly committed by a concurrent verification.
	user, err = scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM users WHERE email_normalized = $1`, email))
	if err != nil {
		return User{}, fmt.Errorf("read existing user: %w", err)
	}
	return user, nil
}

func (s *PostgresStore) AuthenticateSession(ctx context.Context, tokenHash []byte, now, expiresAt time.Time) (User, Session, error) {
	var user User
	var session Session
	err := s.pool.QueryRow(ctx, `
		UPDATE sessions AS session
		SET last_seen_at = $2, expires_at = $3
		FROM users AS app_user
		WHERE session.token_hash = $1
		  AND session.user_id = app_user.id
		  AND session.revoked_at IS NULL
		  AND session.expires_at > $2
		RETURNING session.id::text, session.user_id::text, session.token_hash,
		          session.expires_at, session.last_seen_at, session.created_at,
		          app_user.id::text, app_user.email, app_user.email_normalized,
		          app_user.handle, app_user.email_verified_at, app_user.created_at`,
		tokenHash, now, expiresAt,
	).Scan(
		&session.ID, &session.UserID, &session.TokenHash, &session.ExpiresAt,
		&session.LastSeenAt, &session.CreatedAt, &user.ID, &user.Email,
		&user.EmailNormalized, &user.Handle, &user.EmailVerifiedAt, &user.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, Session{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, Session{}, fmt.Errorf("authenticate session: %w", err)
	}
	return user, session, nil
}

func (s *PostgresStore) RevokeSession(ctx context.Context, tokenHash []byte, now time.Time) error {
	if _, err := s.pool.Exec(ctx, `
		UPDATE sessions SET revoked_at = $2
		WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash, now); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (s *PostgresStore) ClaimHandle(ctx context.Context, userID, handle, normalized string, now time.Time) (User, error) {
	var user User
	err := s.pool.QueryRow(ctx, `
		UPDATE users
		SET handle = $2, handle_normalized = $3, updated_at = $4
		WHERE id = $1
		RETURNING id::text, email, email_normalized, handle, email_verified_at, created_at`,
		userID, handle, normalized, now,
	).Scan(&user.ID, &user.Email, &user.EmailNormalized, &user.Handle, &user.EmailVerifiedAt, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthenticated
	}
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && databaseError.Code == "23505" && databaseError.ConstraintName == "users_handle_normalized_key" {
		return User{}, ErrHandleTaken
	}
	if err != nil {
		return User{}, fmt.Errorf("claim handle: %w", err)
	}
	return user, nil
}
