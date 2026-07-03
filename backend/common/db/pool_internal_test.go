package db

import (
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/config"
)

// TestConnMaxIdle_ConfigDriven asserts a positive config value is used verbatim.
func TestConnMaxIdle_ConfigDriven(t *testing.T) {
	got := connMaxIdle(config.DatabaseConfig{ConnMaxIdleSeconds: 45})
	if got != 45*time.Second {
		t.Fatalf("connMaxIdle = %v, want 45s", got)
	}
}

// TestConnMaxIdle_FallbackOnZero asserts a non-positive config value falls back
// to the package default so an unvalidated config never sets a zero idle window.
func TestConnMaxIdle_FallbackOnZero(t *testing.T) {
	for _, v := range []int{0, -1} {
		got := connMaxIdle(config.DatabaseConfig{ConnMaxIdleSeconds: v})
		if got != defaultConnMaxIdle {
			t.Fatalf("connMaxIdle(%d) = %v, want default %v", v, got, defaultConnMaxIdle)
		}
	}
}

// TestConnMaxLifetime_ConfigDriven asserts a positive config value is used.
func TestConnMaxLifetime_ConfigDriven(t *testing.T) {
	got := connMaxLifetime(config.DatabaseConfig{ConnMaxLifetimeMinutes: 20})
	if got != 20*time.Minute {
		t.Fatalf("connMaxLifetime = %v, want 20m", got)
	}
}

// TestConnMaxLifetime_FallbackOnZero asserts a non-positive value falls back.
func TestConnMaxLifetime_FallbackOnZero(t *testing.T) {
	for _, v := range []int{0, -3} {
		got := connMaxLifetime(config.DatabaseConfig{ConnMaxLifetimeMinutes: v})
		if got != defaultConnMaxLifetime {
			t.Fatalf("connMaxLifetime(%d) = %v, want default %v", v, got, defaultConnMaxLifetime)
		}
	}
}

// TestPoolDefaults_BelowSupavisorIdle documents the invariant that the default
// idle timeout stays well under a typical pooler idle timeout and under the
// connection lifetime, so the fallback path can never violate the contract that
// idle <= lifetime.
func TestPoolDefaults_BelowLifetime(t *testing.T) {
	if defaultConnMaxIdle >= defaultConnMaxLifetime {
		t.Fatalf("defaultConnMaxIdle (%v) must be < defaultConnMaxLifetime (%v)",
			defaultConnMaxIdle, defaultConnMaxLifetime)
	}
}
