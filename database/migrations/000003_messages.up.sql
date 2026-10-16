ALTER TABLE rooms
    ADD CONSTRAINT rooms_id_community_unique UNIQUE (id, community_id);

CREATE TABLE messages (
    id uuid PRIMARY KEY,
    community_id uuid NOT NULL,
    room_id uuid NOT NULL,
    author_member_id uuid NOT NULL,
    client_idempotency_key text NOT NULL,
    body text NOT NULL,
    revision integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    UNIQUE (author_member_id, client_idempotency_key),
    FOREIGN KEY (room_id, community_id) REFERENCES rooms(id, community_id) ON DELETE CASCADE,
    FOREIGN KEY (author_member_id, community_id) REFERENCES members(id, community_id),
    CONSTRAINT messages_body_length CHECK (char_length(body) BETWEEN 1 AND 4000),
    CONSTRAINT messages_idempotency_key_length CHECK (char_length(client_idempotency_key) BETWEEN 8 AND 100),
    CONSTRAINT messages_revision_positive CHECK (revision > 0)
);

CREATE INDEX messages_room_order_idx ON messages (room_id, created_at DESC, id DESC);

CREATE TABLE outbox_events (
    id uuid PRIMARY KEY,
    aggregate_id uuid NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    community_id uuid NOT NULL,
    room_id uuid NOT NULL,
    event_kind text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    published_at timestamptz,
    attempts integer NOT NULL DEFAULT 0,
    FOREIGN KEY (room_id, community_id) REFERENCES rooms(id, community_id) ON DELETE CASCADE,
    CONSTRAINT outbox_event_kind CHECK (event_kind = 'message.created'),
    CONSTRAINT outbox_payload_object CHECK (jsonb_typeof(payload) = 'object'),
    CONSTRAINT outbox_attempts_nonnegative CHECK (attempts >= 0)
);

CREATE INDEX outbox_events_unpublished_idx ON outbox_events (created_at, id) WHERE published_at IS NULL;
