package postgres

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type migration struct {
	version int64
	name    string
	sql     string
}

// Migrate applies every pending *.up.sql file in version order. All pending
// migrations run in one transaction under a transaction-scoped advisory lock,
// so concurrent runners serialize and a failed migration leaves no partial
// schema. The transaction-scoped lock also works through PgBouncer.
func Migrate(ctx context.Context, conn *pgx.Conn, files fs.FS) ([]string, error) {
	migrations, err := loadMigrations(files)
	if err != nil {
		return nil, err
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('skyroom.schema_migrations', 0))`); err != nil {
		return nil, fmt.Errorf("acquire migration lock: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version bigint PRIMARY KEY,
			name text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}

	rows, err := tx.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	applied, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	appliedSet := make(map[int64]struct{}, len(applied))
	for _, version := range applied {
		appliedSet[version] = struct{}{}
	}

	var ran []string
	for _, m := range migrations {
		if _, done := appliedSet[m.version]; done {
			continue
		}
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			return nil, fmt.Errorf("apply migration %s: %w", m.name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.version, m.name); err != nil {
			return nil, fmt.Errorf("record migration %s: %w", m.name, err)
		}
		ran = append(ran, m.name)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit migrations: %w", err)
	}
	return ran, nil
}

func loadMigrations(files fs.FS) ([]migration, error) {
	paths, err := fs.Glob(files, "*.up.sql")
	if err != nil {
		return nil, err
	}
	migrations := make([]migration, 0, len(paths))
	seen := make(map[int64]string, len(paths))
	for _, path := range paths {
		name := strings.TrimSuffix(path, ".up.sql")
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			return nil, fmt.Errorf("migration %q must be named <version>_<name>.up.sql", path)
		}
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("migration %q has an invalid version", path)
		}
		if previous, duplicate := seen[version]; duplicate {
			return nil, fmt.Errorf("migrations %q and %q share version %d", previous, path, version)
		}
		seen[version] = path
		contents, err := fs.ReadFile(files, path)
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, migration{version: version, name: name, sql: string(contents)})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	return migrations, nil
}
