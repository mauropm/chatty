// Package migrations embeds versioned SQL migrations and applies them to a
// PostgreSQL database in order, tracking applied versions in schema_migrations.
package migrations

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed *.sql
var embedFS embed.FS

// Migration is a single versioned up/down migration pair.
type Migration struct {
	Version int64
	Name    string
	Up      string
	Down    string
}

// Load parses all embedded migrations ordered by version.
func Load() ([]Migration, error) {
	entries, err := fs.ReadDir(embedFS, ".")
	if err != nil {
		return nil, fmt.Errorf("migrations: read dir: %w", err)
	}
	byVersion := map[int64]*Migration{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		version, base, kind, err := parseName(name)
		if err != nil {
			return nil, err
		}
		data, err := embedFS.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("migrations: read %s: %w", name, err)
		}
		m := byVersion[version]
		if m == nil {
			m = &Migration{Version: version, Name: base}
			byVersion[version] = m
		}
		if kind == "up" {
			m.Up = string(data)
		} else {
			m.Down = string(data)
		}
	}
	out := make([]Migration, 0, len(byVersion))
	for _, m := range byVersion {
		if m.Up == "" {
			return nil, fmt.Errorf("migrations: version %d missing up migration", m.Version)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// parseName parses "<version>_<name>.<kind>.sql".
func parseName(name string) (int64, string, string, error) {
	trimmed := strings.TrimSuffix(name, ".sql")
	parts := strings.Split(trimmed, ".")
	if len(parts) != 2 {
		return 0, "", "", fmt.Errorf("migrations: bad filename %q", name)
	}
	kind := parts[1]
	if kind != "up" && kind != "down" {
		return 0, "", "", fmt.Errorf("migrations: bad kind %q in %q", kind, name)
	}
	underscore := strings.Index(parts[0], "_")
	if underscore < 0 {
		return 0, "", "", fmt.Errorf("migrations: missing underscore in %q", name)
	}
	version, err := strconv.ParseInt(parts[0][:underscore], 10, 64)
	if err != nil {
		return 0, "", "", fmt.Errorf("migrations: bad version in %q: %w", name, err)
	}
	return version, parts[0][underscore+1:], kind, nil
}

const ensureTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
    version    BIGINT PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`

// AppliedVersions returns the set of versions already applied.
func AppliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[int64]bool, error) {
	if _, err := pool.Exec(ctx, ensureTable); err != nil {
		return nil, fmt.Errorf("migrations: ensure table: %w", err)
	}
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrations: read versions: %w", err)
	}
	defer rows.Close()
	applied := map[int64]bool{}
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// Migrate applies every pending migration, each in its own transaction.
// It returns the versions that were newly applied.
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]int64, error) {
	all, err := Load()
	if err != nil {
		return nil, err
	}
	applied, err := AppliedVersions(ctx, pool)
	if err != nil {
		return nil, err
	}
	var done []int64
	for _, m := range all {
		if applied[m.Version] {
			continue
		}
		if err := applyUp(ctx, pool, m); err != nil {
			return done, err
		}
		done = append(done, m.Version)
	}
	return done, nil
}

func applyUp(ctx context.Context, pool *pgxpool.Pool, m Migration) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migrations: begin %d: %w", m.Version, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, m.Up); err != nil {
		return fmt.Errorf("migrations: apply %d (%s): %w", m.Version, m.Name, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.Version, m.Name); err != nil {
		return fmt.Errorf("migrations: record %d: %w", m.Version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrations: commit %d: %w", m.Version, err)
	}
	return nil
}
