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

func Open(cfg config.DatabaseConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
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

	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	// Prevent stale connections that firewalls or cloud load-balancers silently drop.
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	return db, nil
}

// OpenWithRetry attempts to open a DB connection up to maxAttempts times,
// waiting delay between each attempt. Use this in workers where the database
// may not be ready immediately at process start (e.g. Docker Compose boot order).
// OpenWithRetry attempts to open a DB connection up to maxAttempts times,
// waiting delay between each attempt. Use this in workers where the database
// may not be ready immediately at process start (e.g. Docker Compose boot order).
func OpenWithRetry(cfg config.DatabaseConfig, maxAttempts int, delay time.Duration) (*gorm.DB, error) {
	var lastErr error
	for i := range maxAttempts {
		opened, err := Open(cfg)
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
