package community

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SkyC0der/skyroom/internal/permission"
	platformid "github.com/SkyC0der/skyroom/internal/platform/id"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) CreateCommunity(ctx context.Context, ownerUserID, ownerHandle, name, description string, now time.Time) (CreatedCommunity, error) {
	var created CreatedCommunity
	communityID, err := platformid.New()
	if err != nil {
		return CreatedCommunity{}, err
	}
	memberID, err := platformid.New()
	if err != nil {
		return CreatedCommunity{}, err
	}
	ownerRoleID, err := platformid.New()
	if err != nil {
		return CreatedCommunity{}, err
	}
	everyoneRoleID, err := platformid.New()
	if err != nil {
		return CreatedCommunity{}, err
	}
	roomID, err := platformid.New()
	if err != nil {
		return CreatedCommunity{}, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('community.owner:' || $1, 0))`, ownerUserID); err != nil {
			return fmt.Errorf("lock community owner: %w", err)
		}
		var owned int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM communities WHERE owner_user_id = $1`, ownerUserID).Scan(&owned); err != nil {
			return fmt.Errorf("count owned communities: %w", err)
		}
		if owned >= ownedCommunityLimit {
			return ErrOwnershipLimit
		}

		if err := tx.QueryRow(ctx, `
			INSERT INTO communities (id, owner_user_id, name, description, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $5)
			RETURNING id::text, owner_user_id::text, name, description, created_at`,
			communityID, ownerUserID, name, description, now,
		).Scan(&created.Community.ID, &created.Community.OwnerUserID, &created.Community.Name, &created.Community.Description, &created.Community.CreatedAt); err != nil {
			return fmt.Errorf("insert community: %w", err)
		}
		created.Community.MemberCount = 1

		if err := tx.QueryRow(ctx, `
			INSERT INTO members (id, community_id, user_id, joined_at)
			VALUES ($1, $2, $3, $4)
			RETURNING id::text, community_id::text, user_id::text, joined_at`,
			memberID, created.Community.ID, ownerUserID, now,
		).Scan(&created.Membership.ID, &created.Membership.CommunityID, &created.Membership.UserID, &created.Membership.JoinedAt); err != nil {
			return fmt.Errorf("insert owner membership: %w", err)
		}
		created.Membership.Handle = ownerHandle

		if _, err := tx.Exec(ctx, `
			INSERT INTO roles (id, community_id, name, position, permissions, managed, system_key, created_at)
			VALUES ($1, $2, 'Owner', 1000, $3, true, 'owner', $4)`, ownerRoleID, created.Community.ID, int64(permission.OwnerPermissions), now); err != nil {
			return fmt.Errorf("insert owner role: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO roles (id, community_id, name, position, permissions, managed, system_key, created_at)
			VALUES ($1, $2, 'Everyone', 0, $3, true, 'everyone', $4)`, everyoneRoleID, created.Community.ID, int64(permission.EveryonePermissions), now); err != nil {
			return fmt.Errorf("insert everyone role: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO member_roles (community_id, member_id, role_id, assigned_at)
			VALUES ($1, $2, $3, $5), ($1, $2, $4, $5)`,
			created.Community.ID, created.Membership.ID, ownerRoleID, everyoneRoleID, now); err != nil {
			return fmt.Errorf("assign owner roles: %w", err)
		}

		if err := tx.QueryRow(ctx, `
			INSERT INTO rooms (id, community_id, creator_member_id, kind, name, visibility, created_at)
			VALUES ($1, $2, $3, 'text', 'general', 'members', $4)
			RETURNING id::text, community_id::text, name, kind, visibility, created_at`,
			roomID, created.Community.ID, created.Membership.ID, now,
		).Scan(&created.GeneralRoom.ID, &created.GeneralRoom.CommunityID, &created.GeneralRoom.Name, &created.GeneralRoom.Kind, &created.GeneralRoom.Visibility, &created.GeneralRoom.CreatedAt); err != nil {
			return fmt.Errorf("insert general room: %w", err)
		}
		return nil
	})
	return created, err
}

func (s *PostgresStore) ListCommunities(ctx context.Context, userID string) ([]Community, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id::text, c.owner_user_id::text, c.name, c.description, c.created_at,
		       (SELECT count(*) FROM members count_member WHERE count_member.community_id = c.id AND count_member.state = 'active')
		FROM communities c
		JOIN members member ON member.community_id = c.id
		WHERE member.user_id = $1 AND member.state = 'active'
		ORDER BY c.created_at, c.id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list communities: %w", err)
	}
	defer rows.Close()
	communities := make([]Community, 0)
	for rows.Next() {
		var community Community
		if err := rows.Scan(&community.ID, &community.OwnerUserID, &community.Name, &community.Description, &community.CreatedAt, &community.MemberCount); err != nil {
			return nil, fmt.Errorf("scan community: %w", err)
		}
		communities = append(communities, community)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list communities: %w", err)
	}
	return communities, nil
}

func (s *PostgresStore) CreateInvitation(ctx context.Context, userID, communityID string, tokenHash []byte, expiresAt time.Time, maxUses *int, now time.Time) (Invitation, error) {
	var invitation Invitation
	invitationID, err := platformid.New()
	if err != nil {
		return Invitation{}, err
	}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO invitations (id, community_id, inviter_member_id, token_hash, expires_at, max_uses, created_at)
		SELECT $3, community.id, member.id, $4, $5, $6, $7
		FROM communities community
		JOIN members member ON member.community_id = community.id AND member.user_id = $1 AND member.state = 'active'
		WHERE community.id = $2 AND community.owner_user_id = $1
		RETURNING id::text, community_id::text, expires_at, max_uses, uses_count, revoked_at, created_at`,
		userID, communityID, invitationID, tokenHash, expiresAt, maxUses, now,
	).Scan(&invitation.ID, &invitation.CommunityID, &invitation.ExpiresAt, &invitation.MaxUses, &invitation.UsesCount, &invitation.RevokedAt, &invitation.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, ErrNotAuthorized
	}
	if err != nil {
		return Invitation{}, fmt.Errorf("create invitation: %w", err)
	}
	return invitation, nil
}

func (s *PostgresStore) PreviewInvitation(ctx context.Context, tokenHash []byte, now time.Time) (InvitationPreview, error) {
	var preview InvitationPreview
	var revokedAt *time.Time
	var maxUses *int
	var usesCount int
	err := s.pool.QueryRow(ctx, `
		SELECT community.id::text, community.name, community.description,
		       (SELECT count(*) FROM members count_member WHERE count_member.community_id = community.id AND count_member.state = 'active'),
		       inviter.handle, invitation.expires_at, invitation.revoked_at, invitation.max_uses, invitation.uses_count
		FROM invitations invitation
		JOIN communities community ON community.id = invitation.community_id
		JOIN members inviter_member ON inviter_member.id = invitation.inviter_member_id
		JOIN users inviter ON inviter.id = inviter_member.user_id
		WHERE invitation.token_hash = $1`, tokenHash,
	).Scan(&preview.CommunityID, &preview.CommunityName, &preview.CommunityDescription, &preview.MemberCount, &preview.InviterHandle, &preview.ExpiresAt, &revokedAt, &maxUses, &usesCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return InvitationPreview{}, ErrInvalidInvitation
	}
	if err != nil {
		return InvitationPreview{}, fmt.Errorf("preview invitation: %w", err)
	}
	if revokedAt != nil {
		return InvitationPreview{}, ErrInvitationRevoked
	}
	if !now.Before(preview.ExpiresAt) {
		return InvitationPreview{}, ErrInvitationExpired
	}
	if maxUses != nil && usesCount >= *maxUses {
		return InvitationPreview{}, ErrInvitationExhausted
	}
	return preview, nil
}

func (s *PostgresStore) JoinInvitation(ctx context.Context, userID, handle string, tokenHash []byte, now time.Time) (JoinResult, error) {
	var result JoinResult
	memberID, err := platformid.New()
	if err != nil {
		return JoinResult{}, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var invitationID string
		var revokedAt *time.Time
		var maxUses *int
		var usesCount int
		var expiresAt time.Time
		if err := tx.QueryRow(ctx, `
			SELECT invitation.id::text, community.id::text, community.owner_user_id::text,
			       community.name, community.description, community.created_at,
			       invitation.expires_at, invitation.revoked_at, invitation.max_uses, invitation.uses_count
			FROM invitations invitation
			JOIN communities community ON community.id = invitation.community_id
			WHERE invitation.token_hash = $1
			FOR UPDATE OF invitation`, tokenHash,
		).Scan(&invitationID, &result.Community.ID, &result.Community.OwnerUserID, &result.Community.Name, &result.Community.Description, &result.Community.CreatedAt, &expiresAt, &revokedAt, &maxUses, &usesCount); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalidInvitation
			}
			return fmt.Errorf("lock invitation: %w", err)
		}

		err := tx.QueryRow(ctx, `
			SELECT member.id::text, member.community_id::text, member.user_id::text, app_user.handle, member.joined_at
			FROM members member JOIN users app_user ON app_user.id = member.user_id
			WHERE member.community_id = $1 AND member.user_id = $2`, result.Community.ID, userID,
		).Scan(&result.Membership.ID, &result.Membership.CommunityID, &result.Membership.UserID, &result.Membership.Handle, &result.Membership.JoinedAt)
		if err == nil {
			result.AlreadyMember = true
			return tx.QueryRow(ctx, `SELECT count(*) FROM members WHERE community_id = $1 AND state = 'active'`, result.Community.ID).Scan(&result.Community.MemberCount)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check existing membership: %w", err)
		}
		if revokedAt != nil {
			return ErrInvitationRevoked
		}
		if !now.Before(expiresAt) {
			return ErrInvitationExpired
		}
		if maxUses != nil && usesCount >= *maxUses {
			return ErrInvitationExhausted
		}

		if err := tx.QueryRow(ctx, `
			INSERT INTO members (id, community_id, user_id, joined_at)
			VALUES ($1, $2, $3, $4)
			RETURNING id::text, community_id::text, user_id::text, joined_at`,
			memberID, result.Community.ID, userID, now,
		).Scan(&result.Membership.ID, &result.Membership.CommunityID, &result.Membership.UserID, &result.Membership.JoinedAt); err != nil {
			return fmt.Errorf("insert membership: %w", err)
		}
		result.Membership.Handle = handle
		var everyoneRoleID string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM roles WHERE community_id = $1 AND system_key = 'everyone'`, result.Community.ID).Scan(&everyoneRoleID); err != nil {
			return fmt.Errorf("find everyone role: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO member_roles (community_id, member_id, role_id, assigned_at)
			VALUES ($1, $2, $3, $4)`, result.Community.ID, result.Membership.ID, everyoneRoleID, now); err != nil {
			return fmt.Errorf("assign everyone role: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE invitations SET uses_count = uses_count + 1 WHERE id = $1`, invitationID); err != nil {
			return fmt.Errorf("consume invitation use: %w", err)
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM members WHERE community_id = $1 AND state = 'active'`, result.Community.ID).Scan(&result.Community.MemberCount)
	})
	return result, err
}

func (s *PostgresStore) RevokeInvitation(ctx context.Context, userID, communityID, invitationID string, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE invitations invitation SET revoked_at = $4
		FROM communities community
		WHERE invitation.id = $3 AND invitation.community_id = $2
		  AND community.id = invitation.community_id AND community.owner_user_id = $1
		  AND invitation.revoked_at IS NULL`, userID, communityID, invitationID, now)
	if err != nil {
		return fmt.Errorf("revoke invitation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotAuthorized
	}
	return nil
}

func (s *PostgresStore) HasCapability(ctx context.Context, userID, communityID string, capability permission.Capability) (bool, error) {
	var allowed bool
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(bit_or(role.permissions), 0) & $3 <> 0
		FROM members member
		JOIN member_roles assignment ON assignment.member_id = member.id AND assignment.community_id = member.community_id
		JOIN roles role ON role.id = assignment.role_id AND role.community_id = assignment.community_id
		WHERE member.user_id = $1 AND member.community_id = $2 AND member.state = 'active'`,
		userID, communityID, int64(capability),
	).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("evaluate capability: %w", err)
	}
	return allowed, nil
}
