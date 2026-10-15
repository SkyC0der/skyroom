package postgres_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"

	"github.com/SkyC0der/skyroom/database/migrations"
	"github.com/SkyC0der/skyroom/internal/platform/postgres"
	"github.com/SkyC0der/skyroom/internal/platform/postgres/pgtest"
)

func TestMigrateIsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	conn, err := pgx.ConnectConfig(ctx, pgtest.Config(t))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)

	first, err := postgres.Migrate(ctx, conn, migrations.FS)
	if err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if len(first) == 0 || first[0] != "000001_identity" {
		t.Fatalf("expected identity migration to apply first, got %v", first)
	}
	second, err := postgres.Migrate(ctx, conn, migrations.FS)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("expected no pending migrations, got %v", second)
	}
}

func TestFailedMigrationLeavesNoPartialSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	conn, err := pgx.ConnectConfig(ctx, pgtest.Config(t))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)

	files := fstest.MapFS{
		"000001_ok.up.sql":     {Data: []byte("CREATE TABLE first_table (id int);")},
		"000002_broken.up.sql": {Data: []byte("CREATE TABLE second_table (id int); SELECT broken syntax here;")},
	}
	if _, err := postgres.Migrate(ctx, conn, files); err == nil {
		t.Fatal("expected broken migration to fail")
	}
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass('first_table') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("expected the whole migration run to roll back")
	}
}

func TestMigrateRejectsDuplicateVersions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	conn, err := pgx.ConnectConfig(ctx, pgtest.Config(t))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)

	files := fstest.MapFS{
		"000001_a.up.sql": {Data: []byte("SELECT 1;")},
		"1_b.up.sql":      {Data: []byte("SELECT 1;")},
	}
	if _, err := postgres.Migrate(ctx, conn, files); err == nil {
		t.Fatal("expected duplicate versions to be rejected")
	}
}
