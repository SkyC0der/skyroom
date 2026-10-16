package community

import (
	"context"
	"encoding/base64"
	"sort"
	"sync"
	"time"

	"github.com/SkyC0der/skyroom/internal/permission"
	platformid "github.com/SkyC0der/skyroom/internal/platform/id"
)

type memoryInvitation struct {
	Invitation
	TokenKey      string
	InviterUserID string
	InviterHandle string
}

type MemoryStore struct {
	mu          sync.Mutex
	communities map[string]Community
	members     map[string]map[string]Member
	permissions map[string]map[string]permission.Capability
	invitations map[string]*memoryInvitation
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		communities: make(map[string]Community), members: make(map[string]map[string]Member),
		permissions: make(map[string]map[string]permission.Capability), invitations: make(map[string]*memoryInvitation),
	}
}

func (s *MemoryStore) CreateCommunity(_ context.Context, ownerUserID, ownerHandle, name, description string, now time.Time) (CreatedCommunity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	owned := 0
	for _, existing := range s.communities {
		if existing.OwnerUserID == ownerUserID {
			owned++
		}
	}
	if owned >= ownedCommunityLimit {
		return CreatedCommunity{}, ErrOwnershipLimit
	}
	communityID, err := platformid.New()
	if err != nil {
		return CreatedCommunity{}, err
	}
	memberID, err := platformid.New()
	if err != nil {
		return CreatedCommunity{}, err
	}
	roomID, err := platformid.New()
	if err != nil {
		return CreatedCommunity{}, err
	}
	community := Community{ID: communityID, OwnerUserID: ownerUserID, Name: name, Description: description, MemberCount: 1, CreatedAt: now}
	member := Member{ID: memberID, CommunityID: communityID, UserID: ownerUserID, Handle: ownerHandle, JoinedAt: now}
	room := Room{ID: roomID, CommunityID: communityID, Name: "general", Kind: "text", Visibility: "members", CreatedAt: now}
	s.communities[communityID] = community
	s.members[communityID] = map[string]Member{ownerUserID: member}
	s.permissions[communityID] = map[string]permission.Capability{ownerUserID: permission.OwnerPermissions}
	return CreatedCommunity{Community: community, Membership: member, GeneralRoom: room}, nil
}

func (s *MemoryStore) ListCommunities(_ context.Context, userID string) ([]Community, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Community, 0)
	for communityID, members := range s.members {
		if _, ok := members[userID]; ok {
			community := s.communities[communityID]
			community.MemberCount = len(members)
			result = append(result, community)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (s *MemoryStore) CreateInvitation(_ context.Context, userID, communityID string, tokenHash []byte, expiresAt time.Time, maxUses *int, now time.Time) (Invitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	community, ok := s.communities[communityID]
	if !ok || community.OwnerUserID != userID {
		return Invitation{}, ErrNotAuthorized
	}
	member := s.members[communityID][userID]
	id, err := platformid.New()
	if err != nil {
		return Invitation{}, err
	}
	invitation := Invitation{ID: id, CommunityID: communityID, ExpiresAt: expiresAt, MaxUses: maxUses, CreatedAt: now}
	key := base64.RawURLEncoding.EncodeToString(tokenHash)
	s.invitations[key] = &memoryInvitation{Invitation: invitation, TokenKey: key, InviterUserID: userID, InviterHandle: member.Handle}
	return invitation, nil
}

func (s *MemoryStore) PreviewInvitation(_ context.Context, tokenHash []byte, now time.Time) (InvitationPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	invitation, err := s.activeInvitation(tokenHash, now)
	if err != nil {
		return InvitationPreview{}, err
	}
	community := s.communities[invitation.CommunityID]
	return InvitationPreview{
		CommunityID: community.ID, CommunityName: community.Name, CommunityDescription: community.Description,
		MemberCount: len(s.members[community.ID]), InviterHandle: invitation.InviterHandle, ExpiresAt: invitation.ExpiresAt,
	}, nil
}

func (s *MemoryStore) JoinInvitation(_ context.Context, userID, handle string, tokenHash []byte, now time.Time) (JoinResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := base64.RawURLEncoding.EncodeToString(tokenHash)
	invitation, ok := s.invitations[key]
	if !ok {
		return JoinResult{}, ErrInvalidInvitation
	}
	community := s.communities[invitation.CommunityID]
	if member, exists := s.members[community.ID][userID]; exists {
		community.MemberCount = len(s.members[community.ID])
		return JoinResult{Community: community, Membership: member, AlreadyMember: true}, nil
	}
	if _, err := s.activeInvitation(tokenHash, now); err != nil {
		return JoinResult{}, err
	}
	memberID, err := platformid.New()
	if err != nil {
		return JoinResult{}, err
	}
	member := Member{ID: memberID, CommunityID: community.ID, UserID: userID, Handle: handle, JoinedAt: now}
	s.members[community.ID][userID] = member
	s.permissions[community.ID][userID] = permission.EveryonePermissions
	invitation.UsesCount++
	community.MemberCount = len(s.members[community.ID])
	s.communities[community.ID] = community
	return JoinResult{Community: community, Membership: member}, nil
}

func (s *MemoryStore) RevokeInvitation(_ context.Context, userID, communityID, invitationID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	community, ok := s.communities[communityID]
	if !ok || community.OwnerUserID != userID {
		return ErrNotAuthorized
	}
	for _, invitation := range s.invitations {
		if invitation.ID == invitationID && invitation.CommunityID == communityID && invitation.RevokedAt == nil {
			invitation.RevokedAt = &now
			return nil
		}
	}
	return ErrNotAuthorized
}

func (s *MemoryStore) HasCapability(_ context.Context, userID, communityID string, capability permission.Capability) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.permissions[communityID][userID]&capability != 0, nil
}

func (s *MemoryStore) activeInvitation(tokenHash []byte, now time.Time) (*memoryInvitation, error) {
	key := base64.RawURLEncoding.EncodeToString(tokenHash)
	invitation, ok := s.invitations[key]
	if !ok {
		return nil, ErrInvalidInvitation
	}
	if invitation.RevokedAt != nil {
		return nil, ErrInvitationRevoked
	}
	if !now.Before(invitation.ExpiresAt) {
		return nil, ErrInvitationExpired
	}
	if invitation.MaxUses != nil && invitation.UsesCount >= *invitation.MaxUses {
		return nil, ErrInvitationExhausted
	}
	return invitation, nil
}
