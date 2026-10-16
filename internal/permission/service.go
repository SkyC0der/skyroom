package permission

import "context"

type Capability uint64

const (
	ViewRoom Capability = 1 << iota
	SendMessage
	CreateRoom
)

const (
	OwnerPermissions    = ViewRoom | SendMessage | CreateRoom
	EveryonePermissions = ViewRoom | SendMessage
)

type Store interface {
	HasCapability(ctx context.Context, userID, communityID string, capability Capability) (bool, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Authorize(ctx context.Context, userID, communityID string, capability Capability) (bool, error) {
	return s.store.HasCapability(ctx, userID, communityID, capability)
}
