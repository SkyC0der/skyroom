package community

import (
	"context"
	"errors"
	"testing"

	"github.com/SkyC0der/skyroom/internal/identity"
	"github.com/SkyC0der/skyroom/internal/platform/postgres/pgtest"
)

func TestPostgresCommunityTransactionAndInvitationJoin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := pgtest.MigratedPool(t)
	sender := &testSender{}
	auth := identity.NewService(identity.NewPostgresStore(pool), sender, "test-secret")
	store := NewPostgresStore(pool)
	service := NewService(store, auth)

	ownerToken := verifiedUser(t, auth, sender, "database-owner@example.com", "db.owner")
	created, err := service.Create(ctx, ownerToken, "Database Guild", "Transactionally complete.")
	if err != nil {
		t.Fatal(err)
	}
	var members, roles, assignments, rooms int
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM members WHERE community_id = $1),
		       (SELECT count(*) FROM roles WHERE community_id = $1),
		       (SELECT count(*) FROM member_roles WHERE community_id = $1),
		       (SELECT count(*) FROM rooms WHERE community_id = $1)`, created.Community.ID,
	).Scan(&members, &roles, &assignments, &rooms); err != nil {
		t.Fatal(err)
	}
	if members != 1 || roles != 2 || assignments != 2 || rooms != 1 {
		t.Fatalf("community aggregate was incomplete: members=%d roles=%d assignments=%d rooms=%d", members, roles, assignments, rooms)
	}
	if created.Community.GeneralRoomID != created.GeneralRoom.ID {
		t.Fatalf("expected Community read model to expose general Room ID, community=%+v room=%+v", created.Community, created.GeneralRoom)
	}
	listedRooms, err := service.ListRooms(ctx, ownerToken, created.Community.ID)
	if err != nil || len(listedRooms) != 1 || listedRooms[0].ID != created.GeneralRoom.ID {
		t.Fatalf("expected authorized Room listing, rooms=%+v err=%v", listedRooms, err)
	}

	var ownerRoleID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM roles WHERE community_id = $1 AND system_key = 'owner'`, created.Community.ID).Scan(&ownerRoleID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, ownerRoleID); err == nil {
		t.Fatal("expected the Owner role deletion guard to reject deletion")
	}

	maxUses := 1
	invitation, err := service.CreateInvitation(ctx, ownerToken, created.Community.ID, &maxUses)
	if err != nil {
		t.Fatal(err)
	}
	memberToken := verifiedUser(t, auth, sender, "database-member@example.com", "db.member")
	joined, err := service.Join(ctx, memberToken, invitation.Token)
	if err != nil {
		t.Fatal(err)
	}
	if joined.Community.MemberCount != 2 {
		t.Fatalf("expected two members, got %+v", joined.Community)
	}
	listedRooms, err = service.ListRooms(ctx, memberToken, created.Community.ID)
	if err != nil || len(listedRooms) != 1 || listedRooms[0].ID != created.GeneralRoom.ID {
		t.Fatalf("expected joined member Room listing, rooms=%+v err=%v", listedRooms, err)
	}
	outsiderToken := verifiedUser(t, auth, sender, "database-outsider@example.com", "db.outsider")
	listedRooms, err = service.ListRooms(ctx, outsiderToken, created.Community.ID)
	if err != nil || len(listedRooms) != 0 {
		t.Fatalf("expected outsider Room listing to be empty, rooms=%+v err=%v", listedRooms, err)
	}
	var everyoneAssignments int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM member_roles assignment
		JOIN roles role ON role.id = assignment.role_id
		WHERE assignment.member_id = $1 AND role.system_key = 'everyone'`, joined.Membership.ID,
	).Scan(&everyoneAssignments); err != nil {
		t.Fatal(err)
	}
	if everyoneAssignments != 1 {
		t.Fatalf("expected one Everyone assignment, got %d", everyoneAssignments)
	}
	if allowed, err := service.CanCreateRoom(ctx, memberToken, created.Community.ID); err != nil || allowed {
		t.Fatalf("expected joined member to lack CREATE_ROOM, allowed=%v err=%v", allowed, err)
	}
	if repeated, err := service.Join(ctx, memberToken, invitation.Token); err != nil || !repeated.AlreadyMember {
		t.Fatalf("expected idempotent repeat after max use, result=%+v err=%v", repeated, err)
	}
	if _, err := service.PreviewInvitation(ctx, invitation.Token); !errors.Is(err, ErrInvitationExhausted) {
		t.Fatalf("expected invitation to be exhausted, got %v", err)
	}
}
