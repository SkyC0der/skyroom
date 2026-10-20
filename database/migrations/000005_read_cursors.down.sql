DROP TABLE IF EXISTS read_cursors;

ALTER TABLE messages
    DROP CONSTRAINT IF EXISTS messages_id_room_community_unique;
