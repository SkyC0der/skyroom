ALTER TABLE messages
    ADD CONSTRAINT messages_id_community_room_unique UNIQUE (id, community_id, room_id);

ALTER TABLE outbox_events
    DROP CONSTRAINT outbox_events_aggregate_id_fkey,
    ADD CONSTRAINT outbox_events_aggregate_scope_fkey
        FOREIGN KEY (aggregate_id, community_id, room_id)
        REFERENCES messages(id, community_id, room_id)
        ON DELETE CASCADE;
