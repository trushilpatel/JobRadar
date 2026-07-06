package main

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
)

// orchestratorSpecTitle/orchestratorSpecVersion identify the served OpenAPI
// document's info block, independent of apps/api's own "jobradar-api"
// document — this is a genuinely separate binary/process/port with its own
// middleware baseline (see docs/superpowers/plans/2026-06-13-huma-migration-plan.md,
// Section 9), not a 13th slice merged into apps/api's spec.
const (
	orchestratorSpecTitle   = "jobradar-orchestrator"
	orchestratorSpecVersion = "1.0.0"
)

// orchestratorSchemaRegistry is shared across both of this binary's zones
// (node, admin) so component schemas are never fragmented — mirrors
// apispec.sharedRegistry's rationale exactly (see apihuma.NewSharedRegistry).
var orchestratorSchemaRegistry = apihuma.NewSharedRegistry()

// nodeZoneConfig builds a fresh huma.Config for the /node middleware zone
// (gated by RequireNodeToken). Call once per process and thread the
// returned value through: NodeRestRouter registers its operations onto it.
func nodeZoneConfig() huma.Config {
	return apihuma.NewZoneConfig(orchestratorSpecTitle, orchestratorSpecVersion, orchestratorSchemaRegistry)
}

// adminZoneConfig builds a fresh huma.Config for the /admin/crawler-nodes
// middleware zone (gated by wireAdminGate). Call once per process.
func adminZoneConfig() huma.Config {
	return apihuma.NewZoneConfig(orchestratorSpecTitle, orchestratorSpecVersion, orchestratorSchemaRegistry)
}

// buildOrchestratorSpec merges both zones' operations into the combined
// document served at /openapi.json and /docs. The two zone prefixes
// ("/node", "/admin/crawler-nodes") are fixed, distinct literals, so
// MergeSpecs's only failure mode — a duplicate prefixed path across zones —
// can never occur here; panicking on it (rather than threading an
// unreachable error return through every caller) reflects that this would
// only ever fire on a programming error, never a runtime condition.
func buildOrchestratorSpec(nodeZoneCfg, adminZoneCfg huma.Config) *huma.OpenAPI {
	spec, err := apihuma.MergeSpecs(orchestratorSpecTitle, orchestratorSpecVersion, orchestratorSchemaRegistry,
		apihuma.ZoneSpec{Prefix: "/node", OpenAPI: nodeZoneCfg.OpenAPI},
		apihuma.ZoneSpec{Prefix: "/admin/crawler-nodes", OpenAPI: adminZoneCfg.OpenAPI},
	)
	if err != nil {
		panic("apps/orchestrator: unreachable — /node and /admin/crawler-nodes prefixes can never collide: " + err.Error())
	}
	return spec
}
