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

// Build constructs the current combined OpenAPI spec from every migrated
// zone's ZoneSpec. No zone has migrated yet — main.go passes zero ZoneSpecs
// until Slice 1 (company public) lands, then one more per slice as each
// migrates (docs/superpowers/plans/2026-06-13-huma-migration-plan.md, Section 5).
func Build(zones ...apihuma.ZoneSpec) (*huma.OpenAPI, error) {
	return apihuma.MergeSpecs("jobradar-api", Version, sharedRegistry, zones...)
}
