ALTER TABLE messages
    ADD COLUMN edited_at timestamptz,
    ADD COLUMN deleted_at timestamptz;

ALTER TABLE messages DROP CONSTRAINT messages_body_length;
ALTER TABLE messages ADD CONSTRAINT messages_body_state CHECK (
    (deleted_at IS NULL AND char_length(body) BETWEEN 1 AND 4000) OR
    (deleted_at IS NOT NULL AND body = '')
);

CREATE TABLE message_revisions (
    message_id uuid NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    revision integer NOT NULL,
    body text NOT NULL,
    action text NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (message_id, revision),
    CONSTRAINT message_revisions_revision_positive CHECK (revision > 0),
    CONSTRAINT message_revisions_action CHECK (action IN ('created', 'updated', 'deleted')),
    CONSTRAINT message_revisions_body_state CHECK (
        (action IN ('created', 'updated') AND char_length(body) BETWEEN 1 AND 4000) OR
        (action = 'deleted' AND body = '')
    )
);

INSERT INTO message_revisions (message_id, revision, body, action, created_at)
SELECT id, revision, body, 'created', created_at FROM messages;

ALTER TABLE outbox_events DROP CONSTRAINT outbox_event_kind;
ALTER TABLE outbox_events ADD CONSTRAINT outbox_event_kind
    CHECK (event_kind IN ('message.created', 'message.updated', 'message.deleted'));
