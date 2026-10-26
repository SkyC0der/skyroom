DELETE FROM outbox_events WHERE event_kind IN ('message.updated', 'message.deleted');
ALTER TABLE outbox_events DROP CONSTRAINT outbox_event_kind;
ALTER TABLE outbox_events ADD CONSTRAINT outbox_event_kind CHECK (event_kind = 'message.created');

ALTER TABLE messages DROP CONSTRAINT messages_body_state;
UPDATE messages message SET body = (
    SELECT revision.body
    FROM message_revisions revision
    WHERE revision.message_id = message.id AND revision.body <> ''
    ORDER BY revision.revision DESC
    LIMIT 1
)
WHERE message.deleted_at IS NOT NULL;
DROP TABLE message_revisions;
ALTER TABLE messages ADD CONSTRAINT messages_body_length CHECK (char_length(body) BETWEEN 1 AND 4000);
ALTER TABLE messages DROP COLUMN deleted_at, DROP COLUMN edited_at;
