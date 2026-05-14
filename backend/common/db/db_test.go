package db_test

import (
	"strings"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/db"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newSqliteDB returns a *gorm.DB backed by an in-memory sqlite instance so
// the connection-pool helpers (Ping, PoolReady) can be exercised without a
// running Postgres. Open() itself can only be tested against real Postgres
// or the testcontainers integration tier.
func newSqliteDB(t *testing.T) *gorm.DB {
	t.Helper()
	g, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return g
}

func TestPing_OK(t *testing.T) {
	g := newSqliteDB(t)
	if err := db.Ping(g); err != nil {
		t.Fatalf("Ping on live sqlite: %v", err)
	}
}

func TestPing_AfterClose(t *testing.T) {
	g := newSqliteDB(t)
	sqlDB, err := g.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := db.Ping(g); err == nil {
		t.Fatal("expected Ping error after close, got nil")
	}
}

func TestPoolReady_OK(t *testing.T) {
	g := newSqliteDB(t)
	if err := db.PoolReady(g); err != nil {
		t.Fatalf("PoolReady on live sqlite: %v", err)
	}
}

func TestOpenWithRetry_FailsAfterMaxAttempts(t *testing.T) {
	// An obviously unreachable Postgres host with a tight retry budget keeps
	// the test under ~150ms while still exercising the retry loop.
	cfg := config.DatabaseConfig{
		Host:     "127.0.0.1",
		Port:     "1", // privileged port nothing listens on
		User:     "x",
		Password: "x",
		Name:     "x",
		SSLMode:  "disable",
	}
	start := time.Now()
	_, err := db.OpenWithRetry(cfg, 1, 2, 10*time.Millisecond)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error from unreachable Postgres")
	}
	if !strings.Contains(err.Error(), "after 2 attempts") {
		t.Fatalf("error should mention attempt count, got %q", err.Error())
	}
	// Loose lower bound — at least one inter-attempt sleep should have happened.
	if elapsed < 10*time.Millisecond {
		t.Fatalf("OpenWithRetry returned too fast (%v) — retry loop may not have run", elapsed)
	}
}

func TestOpenWithRetry_SingleAttempt(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host: "127.0.0.1", Port: "1", User: "x", Password: "x", Name: "x", SSLMode: "disable",
	}
	_, err := db.OpenWithRetry(cfg, 1, 1, time.Millisecond)
	if err == nil {
		t.Fatal("expected error with 1 attempt against unreachable host")
	}
	if !strings.Contains(err.Error(), "after 1 attempts") {
		t.Fatalf("error should mention 'after 1 attempts', got %q", err.Error())
	}
}
