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

// Build constructs the current combined OpenAPI spec from every migrated
// zone. No zone has migrated to Huma yet (docs/superpowers/plans/2026-06-13-huma-migration-plan.md,
// Section 5) — each slice adds its apihuma.ZoneSpec to the call below as it
// lands, in migration order.
func Build() (*huma.OpenAPI, error) {
	return apihuma.MergeSpecs("jobradar-api", Version, apihuma.NewSharedRegistry())
}
