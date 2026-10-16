DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS messages;
ALTER TABLE rooms DROP CONSTRAINT IF EXISTS rooms_id_community_unique;
