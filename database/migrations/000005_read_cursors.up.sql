ALTER TABLE messages
    ADD CONSTRAINT messages_id_room_community_unique UNIQUE (id, room_id, community_id);

CREATE TABLE read_cursors (
    member_id uuid NOT NULL,
    community_id uuid NOT NULL,
    room_id uuid NOT NULL,
    last_message_id uuid NOT NULL,
    last_read_created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (member_id, room_id),
    FOREIGN KEY (member_id, community_id) REFERENCES members(id, community_id) ON DELETE CASCADE,
    FOREIGN KEY (room_id, community_id) REFERENCES rooms(id, community_id) ON DELETE CASCADE,
    FOREIGN KEY (last_message_id, room_id, community_id)
        REFERENCES messages(id, room_id, community_id) ON DELETE CASCADE
);

CREATE INDEX read_cursors_room_idx ON read_cursors (room_id, last_read_created_at, last_message_id);
