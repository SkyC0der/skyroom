ALTER TABLE outbox_events
    DROP CONSTRAINT IF EXISTS outbox_events_aggregate_scope_fkey,
    ADD CONSTRAINT outbox_events_aggregate_id_fkey
        FOREIGN KEY (aggregate_id) REFERENCES messages(id) ON DELETE CASCADE;

ALTER TABLE messages
    DROP CONSTRAINT IF EXISTS messages_id_community_room_unique;
