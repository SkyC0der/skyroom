package community

import (
	"context"
	"errors"
	"time"

	"github.com/SkyC0der/skyroom/internal/permission"
)

var (
	ErrInvalidCommunity    = errors.New("invalid community")
	ErrOwnershipLimit      = errors.New("owned community limit reached")
	ErrHandleRequired      = errors.New("global handle required")
	ErrNotAuthorized       = errors.New("not authorized")
	ErrInvalidInvitation   = errors.New("invalid invitation")
	ErrInvitationExpired   = errors.New("invitation expired")
	ErrInvitationRevoked   = errors.New("invitation revoked")
	ErrInvitationExhausted = errors.New("invitation exhausted")
)

const (
	ownedCommunityLimit = 3
	invitationLifetime  = 7 * 24 * time.Hour
)

type Community struct {
	ID          string    `json:"id"`
	OwnerUserID string    `json:"owner_user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	MemberCount int       `json:"member_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type Member struct {
	ID          string    `json:"id"`
	CommunityID string    `json:"community_id"`
	UserID      string    `json:"user_id"`
	Handle      string    `json:"handle"`
	JoinedAt    time.Time `json:"joined_at"`
}

type Room struct {
	ID          string    `json:"id"`
	CommunityID string    `json:"community_id"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	Visibility  string    `json:"visibility"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreatedCommunity struct {
	Community   Community `json:"community"`
	Membership  Member    `json:"membership"`
	GeneralRoom Room      `json:"general_room"`
}

type Invitation struct {
	ID          string     `json:"id"`
	CommunityID string     `json:"community_id"`
	Token       string     `json:"token"`
	ExpiresAt   time.Time  `json:"expires_at"`
	MaxUses     *int       `json:"max_uses"`
	UsesCount   int        `json:"uses_count"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type InvitationPreview struct {
	CommunityID          string    `json:"community_id"`
	CommunityName        string    `json:"community_name"`
	CommunityDescription string    `json:"community_description"`
	MemberCount          int       `json:"member_count"`
	InviterHandle        string    `json:"inviter_handle"`
	ExpiresAt            time.Time `json:"expires_at"`
}

type JoinResult struct {
	Community     Community `json:"community"`
	Membership    Member    `json:"membership"`
	AlreadyMember bool      `json:"already_member"`
}

type Store interface {
	CreateCommunity(ctx context.Context, ownerUserID, ownerHandle, name, description string, now time.Time) (CreatedCommunity, error)
	ListCommunities(ctx context.Context, userID string) ([]Community, error)
	CreateInvitation(ctx context.Context, userID, communityID string, tokenHash []byte, expiresAt time.Time, maxUses *int, now time.Time) (Invitation, error)
	PreviewInvitation(ctx context.Context, tokenHash []byte, now time.Time) (InvitationPreview, error)
	JoinInvitation(ctx context.Context, userID, handle string, tokenHash []byte, now time.Time) (JoinResult, error)
	RevokeInvitation(ctx context.Context, userID, communityID, invitationID string, now time.Time) error
	permission.Store
}
