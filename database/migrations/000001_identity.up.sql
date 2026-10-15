CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL,
    email_normalized text NOT NULL UNIQUE,
    email_verified_at timestamptz NOT NULL,
    handle text,
    handle_normalized text UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_handle_pair CHECK (
        (handle IS NULL AND handle_normalized IS NULL)
        OR (handle IS NOT NULL AND handle_normalized IS NOT NULL)
    )
);

CREATE TABLE email_verification_challenges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email_normalized text NOT NULL,
    code_hash bytea NOT NULL,
    attempts_used smallint NOT NULL DEFAULT 0,
    max_attempts smallint NOT NULL DEFAULT 5,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT verification_attempts_valid CHECK (
        attempts_used >= 0 AND max_attempts > 0 AND attempts_used <= max_attempts
    )
);

CREATE INDEX verification_challenges_email_created_idx
    ON email_verification_challenges (email_normalized, created_at DESC);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sessions_user_active_idx
    ON sessions (user_id, expires_at DESC)
    WHERE revoked_at IS NULL;
