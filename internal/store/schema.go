// Package store is Lotbook's PostgreSQL: its schema, and what the workers
// keep in it.
package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

// migrateLock serializes concurrent runs of Migrate on one database.
const migrateLock = 7_356_201_948

type migration struct {
	version int
	name    string
	sql     string
}

// Migrate brings the schema up to date: it applies, in one transaction, every
// migration not applied yet, and changes nothing when there is none.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	all, err := load()
	if err != nil {
		return err
	}
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, migrateLock); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			CREATE TABLE IF NOT EXISTS schema_migrations (
				version    int         PRIMARY KEY,
				name       text        NOT NULL,
				applied_at timestamptz NOT NULL DEFAULT now()
			)`); err != nil {
			return err
		}
		applied, err := current(ctx, tx)
		if err != nil {
			return err
		}
		for _, m := range all {
			if m.version <= applied {
				continue
			}
			if _, err := tx.Exec(ctx, m.sql); err != nil {
				return fmt.Errorf("migration %04d %s: %w", m.version, m.name, err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.version, m.name); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	return nil
}

// Check fails unless the schema is at the version this binary was built for.
func Check(ctx context.Context, pool *pgxpool.Pool) error {
	all, err := load()
	if err != nil {
		return err
	}
	want := all[len(all)-1].version
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('schema_migrations') IS NOT NULL`).Scan(&exists); err != nil {
		return fmt.Errorf("store: check: %w", err)
	}
	if !exists {
		return fmt.Errorf("store: the schema is not migrated; want version %d; run lotbook migrate", want)
	}
	got, err := current(ctx, pool)
	if err != nil {
		return fmt.Errorf("store: check: %w", err)
	}
	if got != want {
		return fmt.Errorf("store: the schema is at version %d; want %d; run lotbook migrate", got, want)
	}
	return nil
}

func current(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (int, error) {
	var v int
	err := q.QueryRow(ctx, `SELECT coalesce(max(version), 0) FROM schema_migrations`).Scan(&v)
	return v, err
}

// load reads the embedded migrations, named NNNN_name.sql, in version order.
func load() ([]migration, error) {
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	all := make([]migration, 0, len(files))
	for _, f := range files {
		base := strings.TrimSuffix(strings.TrimPrefix(f, "migrations/"), ".sql")
		number, name, ok := strings.Cut(base, "_")
		version, err := strconv.Atoi(number)
		if !ok || err != nil {
			return nil, fmt.Errorf("store: migration %s is not named NNNN_name.sql", f)
		}
		sql, err := migrations.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("store: %w", err)
		}
		all = append(all, migration{version: version, name: name, sql: string(sql)})
	}
	slices.SortFunc(all, func(a, b migration) int { return a.version - b.version })
	if len(all) == 0 {
		return nil, fmt.Errorf("store: no migrations")
	}
	return all, nil
}
