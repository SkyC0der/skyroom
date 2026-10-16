CREATE TABLE communities (
    id uuid PRIMARY KEY,
    owner_user_id uuid NOT NULL REFERENCES users(id),
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    join_policy text NOT NULL DEFAULT 'invite_only',
    permission_version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT communities_name_length CHECK (char_length(name) BETWEEN 2 AND 80),
    CONSTRAINT communities_description_length CHECK (char_length(description) <= 500),
    CONSTRAINT communities_join_policy CHECK (join_policy IN ('invite_only'))
);

CREATE INDEX communities_owner_created_idx ON communities (owner_user_id, created_at DESC);

CREATE TABLE members (
    id uuid PRIMARY KEY,
    community_id uuid NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    display_name text,
    state text NOT NULL DEFAULT 'active',
    joined_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (community_id, user_id),
    UNIQUE (id, community_id),
    CONSTRAINT members_state CHECK (state IN ('active')),
    CONSTRAINT members_display_name_length CHECK (display_name IS NULL OR char_length(display_name) BETWEEN 1 AND 80)
);

CREATE TABLE roles (
    id uuid PRIMARY KEY,
    community_id uuid NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    name text NOT NULL,
    position integer NOT NULL,
    permissions bigint NOT NULL DEFAULT 0,
    managed boolean NOT NULL DEFAULT false,
    system_key text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (community_id, name),
    UNIQUE (community_id, position),
    UNIQUE (community_id, system_key),
    UNIQUE (id, community_id),
    CONSTRAINT roles_permissions_nonnegative CHECK (permissions >= 0),
    CONSTRAINT roles_system_key CHECK (system_key IS NULL OR system_key IN ('owner', 'everyone')),
    CONSTRAINT roles_managed_system_pair CHECK ((system_key IS NULL) OR managed)
);

CREATE FUNCTION protect_system_roles() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' AND OLD.system_key IS NOT NULL THEN
        RAISE EXCEPTION 'system roles cannot be deleted';
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.system_key IS NOT NULL AND
       (NEW.system_key IS DISTINCT FROM OLD.system_key OR NEW.managed IS DISTINCT FROM OLD.managed) THEN
        RAISE EXCEPTION 'system role identity cannot be changed';
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER roles_protect_system_roles
    BEFORE UPDATE OR DELETE ON roles
    FOR EACH ROW EXECUTE FUNCTION protect_system_roles();

CREATE TABLE member_roles (
    community_id uuid NOT NULL,
    member_id uuid NOT NULL,
    role_id uuid NOT NULL,
    assigned_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (member_id, role_id),
    FOREIGN KEY (member_id, community_id) REFERENCES members(id, community_id) ON DELETE CASCADE,
    FOREIGN KEY (role_id, community_id) REFERENCES roles(id, community_id) ON DELETE CASCADE
);

CREATE TABLE rooms (
    id uuid PRIMARY KEY,
    community_id uuid NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    creator_member_id uuid NOT NULL,
    kind text NOT NULL,
    name text NOT NULL,
    visibility text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (community_id, name),
    FOREIGN KEY (creator_member_id, community_id) REFERENCES members(id, community_id),
    CONSTRAINT rooms_kind CHECK (kind IN ('text')),
    CONSTRAINT rooms_visibility CHECK (visibility IN ('members')),
    CONSTRAINT rooms_name_length CHECK (char_length(name) BETWEEN 1 AND 80)
);

CREATE TABLE invitations (
    id uuid PRIMARY KEY,
    community_id uuid NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    inviter_member_id uuid NOT NULL,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    max_uses integer,
    uses_count integer NOT NULL DEFAULT 0,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (inviter_member_id, community_id) REFERENCES members(id, community_id),
    CONSTRAINT invitations_max_uses CHECK (max_uses IS NULL OR max_uses > 0),
    CONSTRAINT invitations_uses_count CHECK (uses_count >= 0 AND (max_uses IS NULL OR uses_count <= max_uses))
);

CREATE INDEX invitations_community_created_idx ON invitations (community_id, created_at DESC);
