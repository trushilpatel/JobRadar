package db

import (
	"fmt"
	"time"

	"github.com/trushilpatel/jobradar/common/config"
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Pool-timeout fallbacks used when a caller passes an unvalidated config with
// non-positive timeout values. They mirror the DatabaseConfig defaults and are
// deliberately below the Supabase Supavisor pooler's idle timeout so the client
// never reuses a connection the pooler has already dropped.
const (
	defaultConnMaxIdle     = 30 * time.Second
	defaultConnMaxLifetime = 15 * time.Minute
)

// connMaxIdle returns the configured idle timeout, falling back to
// defaultConnMaxIdle when the config value is non-positive (unvalidated config).
func connMaxIdle(cfg config.DatabaseConfig) time.Duration {
	if cfg.ConnMaxIdleSeconds <= 0 {
		return defaultConnMaxIdle
	}
	return time.Duration(cfg.ConnMaxIdleSeconds) * time.Second
}

// connMaxLifetime returns the configured connection lifetime, falling back to
// defaultConnMaxLifetime when the config value is non-positive.
func connMaxLifetime(cfg config.DatabaseConfig) time.Duration {
	if cfg.ConnMaxLifetimeMinutes <= 0 {
		return defaultConnMaxLifetime
	}
	return time.Duration(cfg.ConnMaxLifetimeMinutes) * time.Minute
}

// Open connects to Postgres with the supplied DSN and caps the pool at
// maxOpenConns. Idle connections are set equal to maxOpenConns so the full
// pool stays warm under bursty traffic. Callers pick a pool size that fits
// their workload and the shared upstream connection limit.
func Open(cfg config.DatabaseConfig, maxOpenConns int) (*gorm.DB, error) {
	// PreferSimpleProtocol disables server-side prepared statements so the DSN
	// can point at the PgBouncer transaction-mode pooler (port 6543). pgx v5
	// uses extended protocol by default, which transaction mode does not support.
	dialector := postgres.New(postgres.Config{
		DSN:                  cfg.DSN(),
		PreferSimpleProtocol: true,
	})
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	if err := db.Use(otelgorm.NewPlugin()); err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxOpenConns)
	// Prevent stale connections that firewalls or cloud load-balancers silently
	// drop. Config-driven so the idle timeout can be kept below the Supabase
	// Supavisor pooler's own idle timeout — a client idle window that outlives
	// the pooler's leaves the pool handing out already-closed connections,
	// surfacing as mid-flight "unexpected EOF". Falls back to package defaults
	// when the caller passes an unvalidated (zero-value) config.
	sqlDB.SetConnMaxLifetime(connMaxLifetime(cfg))
	sqlDB.SetConnMaxIdleTime(connMaxIdle(cfg))

	return db, nil
}

// OpenWithRetry attempts to open a DB connection up to maxAttempts times,
// waiting delay between each attempt. Use this in workers where the database
// may not be ready immediately at process start (e.g. Docker Compose boot order).
// OpenWithRetry attempts to open a DB connection up to maxAttempts times,
// waiting delay between each attempt. Use this in workers where the database
// may not be ready immediately at process start (e.g. Docker Compose boot order).
func OpenWithRetry(cfg config.DatabaseConfig, maxOpenConns, maxAttempts int, delay time.Duration) (*gorm.DB, error) {
	var lastErr error
	for i := range maxAttempts {
		opened, err := Open(cfg, maxOpenConns)
		if err != nil {
			lastErr = err
		} else if pingErr := Ping(opened); pingErr != nil {
			// Connection opened but not reachable — close it to avoid leaking.
			if sqlDB, e := opened.DB(); e == nil {
				sqlDB.Close() //nolint:errcheck
			}
			lastErr = pingErr
		} else {
			return opened, nil
		}
		if i < maxAttempts-1 {
			time.Sleep(delay)
		}
	}
	return nil, fmt.Errorf("db: failed to connect after %d attempts: %w", maxAttempts, lastErr)
}

// Ping checks that the database connection is alive.
// Used by readiness handlers.
func Ping(database *gorm.DB) error {
	sqlDB, err := database.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

// PoolReady reports whether the connection pool is initialised without
// acquiring a connection. Unlike Ping, this never blocks waiting for a free
// slot, so it is safe to use as a Kubernetes readiness check. Transient pool
// saturation (all connections busy) returns nil — the process is healthy and
// will serve traffic once pressure drops.
func PoolReady(database *gorm.DB) error {
	_, err := database.DB()
	return err
}
