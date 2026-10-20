package community

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/SkyC0der/skyroom/internal/identity"
)

type testSender struct {
	code string
}

func (s *testSender) SendVerificationCode(_ context.Context, _, code string) error {
	s.code = code
	return nil
}

func verifiedUser(t *testing.T, auth *identity.Service, sender *testSender, email, handle string) string {
	t.Helper()
	ctx := context.Background()
	if err := auth.RequestCode(ctx, email); err != nil {
		t.Fatal(err)
	}
	verified, err := auth.VerifyCode(ctx, email, sender.code)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.ClaimHandle(ctx, verified.Token, handle); err != nil {
		t.Fatal(err)
	}
	return verified.Token
}

func TestCreateCommunityInviteAndJoin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sender := &testSender{}
	auth := identity.NewService(identity.NewMemoryStore(), sender, "test-secret")
	store := NewMemoryStore()
	service := NewService(store, auth)

	ownerToken := verifiedUser(t, auth, sender, "owner@example.com", "owner.one")
	created, err := service.Create(ctx, ownerToken, "Builders Guild", "A place to build together.")
	if err != nil {
		t.Fatal(err)
	}
	if created.Community.MemberCount != 1 || created.Community.GeneralRoomID != created.GeneralRoom.ID || created.GeneralRoom.Name != "general" || created.GeneralRoom.Visibility != "members" {
		t.Fatalf("unexpected community aggregate: %+v", created)
	}
	rooms, err := service.ListRooms(ctx, ownerToken, created.Community.ID)
	if err != nil || len(rooms) != 1 || rooms[0].ID != created.GeneralRoom.ID {
		t.Fatalf("expected owner to discover general Room, rooms=%+v err=%v", rooms, err)
	}
	allowed, err := service.CanCreateRoom(ctx, ownerToken, created.Community.ID)
	if err != nil || !allowed {
		t.Fatalf("expected owner CREATE_ROOM capability, allowed=%v err=%v", allowed, err)
	}

	maxUses := 1
	invitation, err := service.CreateInvitation(ctx, ownerToken, created.Community.ID, &maxUses)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewInvitation(ctx, invitation.Token)
	if err != nil {
		t.Fatal(err)
	}
	if preview.CommunityName != "Builders Guild" || preview.MemberCount != 1 || preview.InviterHandle != "owner.one" {
		t.Fatalf("unexpected preview: %+v", preview)
	}

	memberToken := verifiedUser(t, auth, sender, "member@example.com", "member.one")
	joined, err := service.Join(ctx, memberToken, invitation.Token)
	if err != nil {
		t.Fatal(err)
	}
	if joined.AlreadyMember || joined.Community.MemberCount != 2 {
		t.Fatalf("unexpected join result: %+v", joined)
	}
	rooms, err = service.ListRooms(ctx, memberToken, created.Community.ID)
	if err != nil || len(rooms) != 1 || rooms[0].ID != created.GeneralRoom.ID {
		t.Fatalf("expected member to discover general Room, rooms=%+v err=%v", rooms, err)
	}
	outsiderToken := verifiedUser(t, auth, sender, "outsider@example.com", "outsider.one")
	rooms, err = service.ListRooms(ctx, outsiderToken, created.Community.ID)
	if err != nil || len(rooms) != 0 {
		t.Fatalf("expected outsider room discovery to be empty, rooms=%+v err=%v", rooms, err)
	}
	allowed, err = service.CanCreateRoom(ctx, memberToken, created.Community.ID)
	if err != nil || allowed {
		t.Fatalf("expected Everyone to lack CREATE_ROOM, allowed=%v err=%v", allowed, err)
	}

	repeated, err := service.Join(ctx, memberToken, invitation.Token)
	if err != nil || !repeated.AlreadyMember || repeated.Membership.ID != joined.Membership.ID {
		t.Fatalf("expected idempotent join, result=%+v err=%v", repeated, err)
	}
	if _, err := service.PreviewInvitation(ctx, invitation.Token); !errors.Is(err, ErrInvitationExhausted) {
		t.Fatalf("expected exhausted invitation, got %v", err)
	}
}

func TestInvitationCanBeRevokedAndOwnershipIsLimited(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sender := &testSender{}
	auth := identity.NewService(identity.NewMemoryStore(), sender, "test-secret")
	service := NewService(NewMemoryStore(), auth)
	ownerToken := verifiedUser(t, auth, sender, "owner@example.com", "owner.two")

	first, err := service.Create(ctx, ownerToken, "First Community", "")
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := service.CreateInvitation(ctx, ownerToken, first.Community.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RevokeInvitation(ctx, ownerToken, first.Community.ID, invitation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PreviewInvitation(ctx, invitation.Token); !errors.Is(err, ErrInvitationRevoked) {
		t.Fatalf("expected revoked invitation, got %v", err)
	}

	for _, name := range []string{"Second Community", "Third Community"} {
		if _, err := service.Create(ctx, ownerToken, name, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Create(ctx, ownerToken, "Fourth Community", ""); !errors.Is(err, ErrOwnershipLimit) {
		t.Fatalf("expected ownership limit, got %v", err)
	}
}

func TestCreateRoomEnforcesCapabilityNamingAndQuota(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sender := &testSender{}
	auth := identity.NewService(identity.NewMemoryStore(), sender, "test-secret")
	store := NewMemoryStore()
	service := NewService(store, auth)
	ownerToken := verifiedUser(t, auth, sender, "room-owner@example.com", "room.owner")
	created, err := service.Create(ctx, ownerToken, "Room Makers", "")
	if err != nil {
		t.Fatal(err)
	}
	room, err := service.CreateRoom(ctx, ownerToken, created.Community.ID, "  Product & Design  ")
	if err != nil {
		t.Fatal(err)
	}
	if room.Name != "product-design" || room.Kind != "text" || room.Visibility != "members" {
		t.Fatalf("unexpected Room: %+v", room)
	}
	if len(store.auditEvents) != 1 || store.auditEvents[0].TargetID != room.ID || store.auditEvents[0].Action != "room.created" {
		t.Fatalf("expected atomic room.created audit event, got %+v", store.auditEvents)
	}
	if _, err := service.CreateRoom(ctx, ownerToken, created.Community.ID, "product design"); !errors.Is(err, ErrRoomNameTaken) {
		t.Fatalf("expected normalized duplicate name rejection, got %v", err)
	}

	invitation, err := service.CreateInvitation(ctx, ownerToken, created.Community.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	memberToken := verifiedUser(t, auth, sender, "room-member@example.com", "room.member")
	if _, err := service.Join(ctx, memberToken, invitation.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateRoom(ctx, memberToken, created.Community.ID, "not-allowed"); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("expected CREATE_ROOM denial, got %v", err)
	}
	if _, err := service.CreateRoom(ctx, ownerToken, created.Community.ID, "***"); !errors.Is(err, ErrInvalidRoom) {
		t.Fatalf("expected invalid normalized name, got %v", err)
	}

	for index := 0; index < communityRoomLimit-2; index++ {
		if _, err := service.CreateRoom(ctx, ownerToken, created.Community.ID, fmt.Sprintf("room-%02d", index)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.CreateRoom(ctx, ownerToken, created.Community.ID, "over-quota"); !errors.Is(err, ErrRoomLimit) {
		t.Fatalf("expected Room quota, got %v", err)
	}
}
