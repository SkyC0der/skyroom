package community

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"regexp"
	"strings"
	"time"

	"github.com/SkyC0der/skyroom/internal/identity"
	"github.com/SkyC0der/skyroom/internal/permission"
)

var roomNameSeparators = regexp.MustCompile(`[^a-z0-9]+`)

type Authenticator interface {
	Authenticate(context.Context, string) (identity.AuthenticatedSession, error)
}

type Service struct {
	store       Store
	auth        Authenticator
	permissions *permission.Service
	now         func() time.Time
}

func NewService(store Store, auth Authenticator) *Service {
	return &Service{store: store, auth: auth, permissions: permission.NewService(store), now: time.Now}
}

func (s *Service) Create(ctx context.Context, sessionToken, name, description string) (CreatedCommunity, error) {
	authenticated, err := s.auth.Authenticate(ctx, sessionToken)
	if err != nil {
		return CreatedCommunity{}, err
	}
	if authenticated.User.Handle == nil {
		return CreatedCommunity{}, ErrHandleRequired
	}
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if len(name) < 2 || len(name) > 80 || len(description) > 500 {
		return CreatedCommunity{}, ErrInvalidCommunity
	}
	return s.store.CreateCommunity(ctx, authenticated.User.ID, *authenticated.User.Handle, name, description, s.now().UTC())
}

func (s *Service) List(ctx context.Context, sessionToken string) ([]Community, error) {
	authenticated, err := s.auth.Authenticate(ctx, sessionToken)
	if err != nil {
		return nil, err
	}
	return s.store.ListCommunities(ctx, authenticated.User.ID)
}

func (s *Service) ListRooms(ctx context.Context, sessionToken, communityID string) ([]Room, error) {
	authenticated, err := s.auth.Authenticate(ctx, sessionToken)
	if err != nil {
		return nil, err
	}
	return s.store.ListRooms(ctx, authenticated.User.ID, communityID)
}

func (s *Service) CreateRoom(ctx context.Context, sessionToken, communityID, name string) (Room, error) {
	authenticated, err := s.auth.Authenticate(ctx, sessionToken)
	if err != nil {
		return Room{}, err
	}
	name = roomNameSeparators.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	name = strings.Trim(name, "-")
	if len(name) < 1 || len(name) > 80 {
		return Room{}, ErrInvalidRoom
	}
	return s.store.CreateRoom(ctx, authenticated.User.ID, communityID, name, s.now().UTC())
}

func (s *Service) CreateInvitation(ctx context.Context, sessionToken, communityID string, maxUses *int) (Invitation, error) {
	authenticated, err := s.auth.Authenticate(ctx, sessionToken)
	if err != nil {
		return Invitation{}, err
	}
	if maxUses != nil && (*maxUses < 1 || *maxUses > 10_000) {
		return Invitation{}, ErrInvalidInvitation
	}
	token, tokenHash, err := generateInvitationToken()
	if err != nil {
		return Invitation{}, err
	}
	now := s.now().UTC()
	invitation, err := s.store.CreateInvitation(ctx, authenticated.User.ID, communityID, tokenHash, now.Add(invitationLifetime), maxUses, now)
	if err != nil {
		return Invitation{}, err
	}
	invitation.Token = token
	return invitation, nil
}

func (s *Service) PreviewInvitation(ctx context.Context, token string) (InvitationPreview, error) {
	tokenHash, err := hashInvitationToken(token)
	if err != nil {
		return InvitationPreview{}, ErrInvalidInvitation
	}
	return s.store.PreviewInvitation(ctx, tokenHash, s.now().UTC())
}

func (s *Service) Join(ctx context.Context, sessionToken, invitationToken string) (JoinResult, error) {
	authenticated, err := s.auth.Authenticate(ctx, sessionToken)
	if err != nil {
		return JoinResult{}, err
	}
	if authenticated.User.Handle == nil {
		return JoinResult{}, ErrHandleRequired
	}
	tokenHash, err := hashInvitationToken(invitationToken)
	if err != nil {
		return JoinResult{}, ErrInvalidInvitation
	}
	return s.store.JoinInvitation(ctx, authenticated.User.ID, *authenticated.User.Handle, tokenHash, s.now().UTC())
}

func (s *Service) RevokeInvitation(ctx context.Context, sessionToken, communityID, invitationID string) error {
	authenticated, err := s.auth.Authenticate(ctx, sessionToken)
	if err != nil {
		return err
	}
	return s.store.RevokeInvitation(ctx, authenticated.User.ID, communityID, invitationID, s.now().UTC())
}

func (s *Service) CanCreateRoom(ctx context.Context, sessionToken, communityID string) (bool, error) {
	authenticated, err := s.auth.Authenticate(ctx, sessionToken)
	if err != nil {
		return false, err
	}
	return s.permissions.Authorize(ctx, authenticated.User.ID, communityID, permission.CreateRoom)
}

func generateInvitationToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	return token, hash[:], nil
}

func hashInvitationToken(token string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, ErrInvalidInvitation
	}
	hash := sha256.Sum256([]byte(token))
	return hash[:], nil
}
