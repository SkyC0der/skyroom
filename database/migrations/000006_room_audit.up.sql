CREATE TABLE audit_events (
    id uuid PRIMARY KEY,
    community_id uuid NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    actor_member_id uuid NOT NULL,
    action text NOT NULL,
    target_type text NOT NULL,
    target_id uuid NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL,
    FOREIGN KEY (actor_member_id, community_id) REFERENCES members(id, community_id),
    CONSTRAINT audit_events_action CHECK (action IN ('room.created')),
    CONSTRAINT audit_events_target_type CHECK (target_type IN ('room')),
    CONSTRAINT audit_events_metadata_object CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX audit_events_community_created_idx ON audit_events (community_id, created_at DESC, id DESC);
