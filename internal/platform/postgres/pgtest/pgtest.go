// Package pgtest provisions isolated PostgreSQL schemas for integration tests.
// Tests skip unless SKYROOM_TEST_DATABASE_URL points at a disposable database.
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SkyC0der/skyroom/database/migrations"
	"github.com/SkyC0der/skyroom/internal/platform/postgres"
)

const EnvURL = "SKYROOM_TEST_DATABASE_URL"

// Config returns a connection config whose search_path is a fresh schema that
// is dropped when the test ends, so parallel tests never share rows.
func Config(t *testing.T) *pgx.ConnConfig {
	t.Helper()
	url := os.Getenv(EnvURL)
	if url == "" {
		t.Skipf("%s is not set; skipping PostgreSQL integration test", EnvURL)
	}

	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	schema := "test_" + hex.EncodeToString(raw)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	defer admin.Close(ctx)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, url)
		if err != nil {
			t.Errorf("connect to drop test schema: %v", err)
			return
		}
		defer conn.Close(ctx)
		if _, err := conn.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})

	config, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	config.RuntimeParams["search_path"] = schema
	return config
}

// MigratedPool returns a pool on a fresh, fully migrated schema.
func MigratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	connConfig := Config(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := pgx.ConnectConfig(ctx, connConfig)
	if err != nil {
		t.Fatalf("connect for migrations: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := postgres.Migrate(ctx, conn, migrations.FS); err != nil {
		t.Fatalf("migrate test schema: %v", err)
	}

	poolConfig, err := pgxpool.ParseConfig("")
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.ConnConfig = connConfig
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("open test pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
