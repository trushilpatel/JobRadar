// Package apispec is the single source of truth for which middleware zones
// have migrated to Huma and how their combined OpenAPI document is built. It
// is shared by apps/api (serves the spec on the ops listener) and
// apps/genspec (writes it to disk for the stale-spec CI guard) so there is
// exactly one place that knows the current zone list — see
// docs/superpowers/plans/2026-06-13-huma-migration-plan.md, Section 3.5.
package apispec

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
)

// Version is the version reported in the served OpenAPI document's
// info.version field. Bump when the API's public contract changes in a way
// clients should notice — independent of the Go module version.
const Version = "1.0.0"

// sharedRegistry backs every zone config this package builds, so component
// schemas are never fragmented across the /api and /admin zones — see
// apihuma.NewSharedRegistry.
var sharedRegistry = apihuma.NewSharedRegistry()

// APIZoneConfig builds a fresh huma.Config for the /api middleware zone.
// Call this exactly ONCE per process (in main.go) and thread the returned
// value through: every /api-zone slice's RestRouter constructor takes it so
// their operations share one zone document, and the same value's .OpenAPI
// is later passed to Build once all Register calls have populated it.
// Calling this twice produces two independent documents — do not do that.
func APIZoneConfig() huma.Config {
	return apihuma.NewZoneConfig("jobradar-api", Version, sharedRegistry)
}

// AdminZoneConfig builds a fresh huma.Config for the /admin middleware zone.
// Same one-call-per-process rule as APIZoneConfig — every /admin-zone
// slice's RestRouter constructor takes the same returned value so their
// operations share one zone document. Uses the same sharedRegistry as
// APIZoneConfig so component schemas never fragment across zones (see
// apihuma.NewSharedRegistry), while keeping a completely independent Paths
// map — required because /admin routers register relative paths on the
// already-"/admin"-mounted subrouter (see adminRouteGroup.Register), which
// would collide with /api's relative paths if they shared one document.
func AdminZoneConfig() huma.Config {
	return apihuma.NewZoneConfig("jobradar-api", Version, sharedRegistry)
}

// Build constructs the current combined OpenAPI spec from every migrated
// zone's ZoneSpec. No zone has migrated yet — main.go passes zero ZoneSpecs
// until Slice 1 (company public) lands, then one more per slice as each
// migrates (docs/superpowers/plans/2026-06-13-huma-migration-plan.md, Section 5).
func Build(zones ...apihuma.ZoneSpec) (*huma.OpenAPI, error) {
	return apihuma.MergeSpecs("jobradar-api", Version, sharedRegistry, zones...)
}

// ApplyPublicURL sets spec's servers entry to publicURL, a no-op when
// publicURL is empty. Call this on the live-served spec only (main.go),
// never on the spec genspec.go writes to the committed apps/api/openapi.yaml
// — that file must stay environment-agnostic.
//
// The docs UI is served from the ops listener (a different port than the
// real API — see apps/api/main.go), so without an explicit servers entry,
// Stoplight's "Try it" console defaults to sending requests to the docs
// page's own origin instead of the API.
func ApplyPublicURL(spec *huma.OpenAPI, publicURL string) {
	if publicURL == "" {
		return
	}
	spec.Servers = []*huma.Server{{URL: publicURL}}
}
