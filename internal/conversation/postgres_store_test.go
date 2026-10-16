package conversation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/SkyC0der/skyroom/internal/community"
	"github.com/SkyC0der/skyroom/internal/identity"
	platformid "github.com/SkyC0der/skyroom/internal/platform/id"
	"github.com/SkyC0der/skyroom/internal/platform/postgres/pgtest"
)

func TestPostgresMessageAndOutboxCommitAtomically(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := pgtest.MigratedPool(t)
	sender := &testSender{}
	auth := identity.NewService(identity.NewPostgresStore(pool), sender, "test-secret")
	communityService := community.NewService(community.NewPostgresStore(pool), auth)
	service := NewService(NewPostgresStore(pool), auth)
	ownerToken := verifiedUser(t, auth, sender, "postgres-message@example.com", "postgres.message")
	created, err := communityService.Create(ctx, ownerToken, "Postgres Messages", "")
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := communityService.CreateInvitation(ctx, ownerToken, created.Community.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	memberToken := verifiedUser(t, auth, sender, "postgres-member-message@example.com", "postgres.member")
	if _, err := communityService.Join(ctx, memberToken, invitation.Token); err != nil {
		t.Fatal(err)
	}

	posted, err := service.Post(ctx, memberToken, created.GeneralRoom.ID, "postgres-client-key", "Durable message")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := service.Post(ctx, memberToken, created.GeneralRoom.ID, "postgres-client-key", "Durable message")
	if err != nil || !repeated.Duplicate || repeated.Message.ID != posted.Message.ID {
		t.Fatalf("expected idempotent retry, result=%+v err=%v", repeated, err)
	}
	var messageCount, eventCount int
	var payload []byte
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM messages WHERE room_id = $1),
		       (SELECT count(*) FROM outbox_events WHERE room_id = $1),
		       (SELECT payload FROM outbox_events WHERE room_id = $1)`, created.GeneralRoom.ID,
	).Scan(&messageCount, &eventCount, &payload); err != nil {
		t.Fatal(err)
	}
	if messageCount != 1 || eventCount != 1 {
		t.Fatalf("expected one message and event, messages=%d events=%d", messageCount, eventCount)
	}
	var envelope struct {
		Type    string `json:"type"`
		Payload struct {
			Message Message `json:"message"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Type != "message.created" || envelope.Payload.Message.ID != posted.Message.ID {
		t.Fatalf("unexpected outbox envelope: %+v", envelope)
	}
	other, err := communityService.Create(ctx, ownerToken, "Other Postgres Room", "")
	if err != nil {
		t.Fatal(err)
	}
	crossScopeEventID, err := platformid.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_events (id, aggregate_id, community_id, room_id, event_kind, payload, created_at)
		VALUES ($1, $2, $3, $4, 'message.created', '{}', now())`,
		crossScopeEventID, posted.Message.ID, other.Community.ID, other.GeneralRoom.ID); err == nil {
		t.Fatal("expected cross-Community outbox reference to be rejected")
	}

	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION reject_test_outbox() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'forced outbox failure'; END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER reject_test_outbox BEFORE INSERT ON outbox_events
		FOR EACH ROW EXECUTE FUNCTION reject_test_outbox();`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Post(ctx, memberToken, created.GeneralRoom.ID, "rollback-client-key", "Must roll back"); err == nil {
		t.Fatal("expected forced outbox failure")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE body = 'Must roll back'`).Scan(&messageCount); err != nil {
		t.Fatal(err)
	}
	if messageCount != 0 {
		t.Fatalf("expected message rollback, found %d rows", messageCount)
	}
}
