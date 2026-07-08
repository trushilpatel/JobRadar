package main

import (
	"context"
	"fmt"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// printSpecFlag, when passed as the sole CLI argument, switches this binary
// from serving the API to regenerating apps/orchestrator/openapi.yaml and
// exiting. CI runs this so the committed spec can never silently drift from
// what the server actually serves — the same stale-spec guard apps/api uses
// (docs/superpowers/plans/2026-06-13-huma-migration-plan.md, Section 3.5).
//
// This reuses wireApp — the exact same wiring main() calls — rather than
// maintaining a second, parallel list of routers. Route registration never
// queries the database or publishes to NATS, so an in-memory sqlite *gorm.DB
// and a noopPublisher stand in for the real dependencies.
const printSpecFlag = "--print-spec"

// specOutputPath is relative to the backend/ module root, matching how
// `go run ./apps/orchestrator --print-spec` is invoked from CI and local
// tooling. A var (not const) so tests can redirect it to a temp file.
var specOutputPath = "apps/orchestrator/openapi.yaml"

// noopPublisher discards every publish — route registration never calls
// StreamPublisher, so this only exists to satisfy wireApp's constructor.
type noopPublisher struct{}

func (noopPublisher) PublishWithMsgID(context.Context, string, []byte, string) error { return nil }

// buildSpecYAML runs the exact same wiring main() uses (wireApp + both
// RestRouters' Register) against a database-free in-memory sqlite handle and
// a noop NATS publisher, then returns the resulting combined OpenAPI
// document marshaled as YAML.
func buildSpecYAML() ([]byte, error) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open in-memory db: %w", err)
	}

	nodeZoneCfg := nodeZoneConfig()
	adminZoneCfg := adminZoneConfig()

	// authErr = nil: pretend Auth succeeded so the admin surface's real
	// operations — not wireAdminGate's 503 stub — get documented; the spec
	// describes the intended contract, not this process's transient JWKS
	// reachability.
	app := wireApp(db, &config.Config{}, logger.NewNoop(), noopPublisher{}, nil, nodeZoneCfg, adminZoneCfg)

	r := chi.NewRouter()
	r.Route("/node", app.nodeRouter.Register)
	r.Route("/admin/crawler-nodes", app.adminGate.Register)

	spec := buildOrchestratorSpec(nodeZoneCfg, adminZoneCfg)
	return spec.YAML()
}

func printSpec() error {
	yamlBytes, err := buildSpecYAML()
	if err != nil {
		return err
	}
	if err := os.WriteFile(specOutputPath, yamlBytes, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", specOutputPath, err)
	}
	return nil
}
