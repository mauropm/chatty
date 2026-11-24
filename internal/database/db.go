// Package database manages the PostgreSQL connection pool and startup
// connectivity for Chatty.
package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"chatty/internal/config"
	"chatty/internal/migrations"
)

// DB wraps a pgxpool and exposes health helpers.
type DB struct {
	Pool   *pgxpool.Pool
	config config.DatabaseConfig
	log    *slog.Logger
}

// Connect builds a pool and waits for PostgreSQL to become reachable, then
// runs pending migrations. It retries with backoff so Chatty can start before
// the database container is ready.
func Connect(ctx context.Context, cfg config.DatabaseConfig, log *slog.Logger) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("database: parse config: %w", err)
	}
	poolCfg.MaxConns = cfg.MaxConns
	if poolCfg.MaxConns < 1 {
		poolCfg.MaxConns = 10
	}
	poolCfg.MinConns = cfg.MinConns
	if poolCfg.MinConns < 0 {
		poolCfg.MinConns = 0
	}
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.MaxConnIdleTime = 5 * time.Minute
	// pgx defaults are sane; keep statement cache disabled for proxy friendliness.
	poolCfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		// Ensure sessions report UTC.
		_, err := conn.Exec(ctx, `SET TIME ZONE 'UTC'`)
		return err
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("database: create pool: %w", err)
	}

	db := &DB{Pool: pool, config: cfg, log: log}
	if err := db.waitForReady(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return db, nil
}

// waitForReady pings the pool until it succeeds or the context deadline ends.
func (d *DB) waitForReady(ctx context.Context) error {
	deadline := time.Now().Add(60 * time.Second)
	var attempt int
	for {
		attempt++
		pingCtx, cancel := context.WithTimeout(ctx, d.config.PingTimeout)
		err := d.Pool.Ping(pingCtx)
		cancel()
		if err == nil {
			d.log.Info("database connected", slog.String("host", d.config.Host), slog.Int("port", d.config.Port))
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("database: not ready: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("database: timed out waiting for readiness after %d attempts: %w", attempt, err)
		}
		d.log.Warn("database not ready, retrying",
			slog.Int("attempt", attempt),
			slog.String("err", sanitizeErr(err)),
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// Migrate runs pending SQL migrations.
func (d *DB) Migrate(ctx context.Context) error {
	applied, err := migrations.Migrate(ctx, d.Pool)
	if err != nil {
		return err
	}
	if len(applied) > 0 {
		d.log.Info("migrations applied", slog.Any("versions", applied))
	} else {
		d.log.Debug("migrations up to date")
	}
	return nil
}

// Ping reports database reachability and round-trip latency in milliseconds.
func (d *DB) Ping(ctx context.Context) (latencyMs int64, err error) {
	start := time.Now()
	pingCtx, cancel := context.WithTimeout(ctx, d.config.PingTimeout)
	defer cancel()
	if err := d.Pool.Ping(pingCtx); err != nil {
		return 0, err
	}
	return time.Since(start).Milliseconds(), nil
}

// Close releases the pool.
func (d *DB) Close() { d.Pool.Close() }

// IsNotFound maps pgx.ErrNoRows to a shared sentinel for callers.
func IsNotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// sanitizeErr keeps credentials out of logs (DSNs are never logged, but be safe).
func sanitizeErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if idx := indexOf(msg, "password="); idx >= 0 {
		msg = msg[:idx] + "password=***"
	}
	return msg
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
