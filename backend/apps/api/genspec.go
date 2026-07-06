package main

import (
	"fmt"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/apps/api/apispec"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/cache"
	clerkadmin "github.com/trushilpatel/jobradar/platform/clerkadmin"
	"github.com/trushilpatel/jobradar/platform/ratelimit"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// genspecRateLimitBurst/genspecRateLimitRefillPerSec are throwaway values
// for the in-memory rate limiter genspec constructs — route registration
// never actually calls it, so the exact numbers don't matter.
const (
	genspecRateLimitBurst             = 20.0
	genspecRateLimitRefillPerSec      = genspecRateLimitBurst / 60.0
	genspecClerkAPIBaseURLPlaceholder = "https://api.clerk.dev"
)

// printSpecFlag, when passed as the sole CLI argument, switches this binary
// from serving the API to regenerating apps/api/openapi.yaml and exiting.
// CI runs this so the committed spec can never silently drift from what the
// server actually serves — the stale-spec guard
// (docs/superpowers/plans/2026-06-13-huma-migration-plan.md, Section 3.5).
//
// This reuses InitializeApp — the exact same wiring main() calls — rather
// than maintaining a second, parallel list of migrated routers (a second
// list could itself drift from reality). A standalone genspec binary can't
// do this: Go does not allow importing another package main, and apps/<name>
// binaries are wiring-only by convention (apps/CLAUDE.md), so composition
// reuse across binaries isn't how this codebase is structured. Running as a
// mode of apps/api itself sidesteps both problems.
//
// Route registration never queries the database, so an in-memory sqlite
// *gorm.DB stands in for Postgres — nothing is read or written to it.
const printSpecFlag = "--print-spec"

// specOutputPath is relative to the backend/ module root, matching how
// `go run ./apps/api --print-spec` is invoked from CI and local tooling.
// A var (not const) so tests can redirect it to a temp file.
var specOutputPath = "apps/api/openapi.yaml"

// buildSpecYAML runs the exact same wiring main() uses (InitializeApp +
// every /api-zone RestRouter's Register) against a database-free in-memory
// sqlite handle, then returns the resulting combined OpenAPI document
// marshaled as YAML.
func buildSpecYAML() ([]byte, error) {
	// Huma derives the documented error response schema from huma.NewError
	// at operation-registration time (defineErrors calls NewError(0, "") to
	// get a sample value to reflect over) — this must be installed BEFORE
	// InitializeApp/Register runs, exactly like main() does, or the
	// generated docs would show huma's default RFC 9457 shape instead of
	// the legacy envelope the API actually returns.
	apihuma.InstallLegacyErrorModel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open in-memory db: %w", err)
	}

	apiZoneCfg := apispec.APIZoneConfig()

	app, err := InitializeApp(db, &config.Config{}, logger.NewNoop(), apiZoneCfg)
	if err != nil {
		return nil, fmt.Errorf("wire app: %w", err)
	}

	// buildMultiUser wires /api/me (mu.APIHandlers()) and the /admin/* zone
	// (mu.AdminHandlers()) — its cache/limiter/Clerk-client dependencies are
	// only ever used to satisfy constructors here, never actually invoked,
	// since route registration never queries anything real. A genuine Redis
	// or Clerk dependency would make spec generation flaky and network-bound,
	// which it must never be.
	noopCache := cache.NewMemoryCache()
	noopLimiter := ratelimit.NewMemoryLimiter(genspecRateLimitBurst, genspecRateLimitRefillPerSec)
	clerkClient, _ := clerkadmin.NewRESTClient(genspecClerkAPIBaseURLPlaceholder, "", nil)
	mu := buildMultiUser(db, &config.Config{}, noopCache, noopLimiter, clerkClient, logger.NewNoop(),
		app.CompanyAdminHandler(), app.AtsHandler(), app.PipelineHandler(), apiZoneCfg)

	// Register every /api-zone RestRouter directly — no middleware, no
	// listener. Huma-migrated routers populate apiZoneCfg.OpenAPI as a side
	// effect; not-yet-migrated ones register plain chi routes, a no-op here.
	r := chi.NewRouter()
	for _, g := range app.Handlers() {
		g.Register(r)
	}
	for _, g := range mu.APIHandlers() {
		g.Register(r)
	}
	for _, g := range mu.AdminHandlers() {
		g.Register(r)
	}

	spec, err := apispec.Build(apihuma.ZoneSpec{Prefix: "/api", OpenAPI: apiZoneCfg.OpenAPI})
	if err != nil {
		return nil, fmt.Errorf("build spec: %w", err)
	}

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
