package apihuma

import (
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// schemaPrefix and docsSecurityHeaders mirror huma.DefaultConfig's own
// internal choices — kept identical so the served spec/docs look the same
// as any other huma-generated one.
const schemaPrefix = "#/components/schemas/"

// NewSharedRegistry builds one component-schema registry meant to be reused
// across every middleware zone's huma.Config. Sharing it (rather than each
// zone getting its own, as huma.DefaultConfig would do by default) is what
// keeps $ref'd schemas de-duplicated across zones — the Phase 0 spike found
// that zones must keep separate Paths maps (relative-path route registration
// would otherwise collide), but there is no such constraint on schemas, so
// there is no reason to fragment them.
func NewSharedRegistry() huma.Registry {
	return huma.NewMapRegistry(schemaPrefix, huma.DefaultSchemaNamer)
}

// NewZoneConfig builds a huma.Config for one middleware zone (e.g. the
// "/api" or "/admin" mount). All zones must be given the SAME registry
// (see NewSharedRegistry). Each zone's own OpenAPIPath/DocsPath are disabled
// (left blank) — per-zone auto-serving would put spec/docs behind that
// zone's auth middleware, which Phase 0 (Section 3.5) explicitly avoids;
// the combined document is served separately via MountCombinedSpec on the
// unauthenticated ops listener instead.
func NewZoneConfig(title, version string, registry huma.Registry) huma.Config {
	cfg := huma.DefaultConfig(title, version)
	cfg.Components.Schemas = registry
	cfg.OpenAPIPath = ""
	cfg.DocsPath = ""
	return cfg
}

// ZoneSpec pairs a middleware zone's mount prefix with the *huma.OpenAPI it
// accumulated via huma.Register calls during that zone's router setup.
type ZoneSpec struct {
	Prefix  string
	OpenAPI *huma.OpenAPI
}

// MergeSpecs combines every zone's operations into one *huma.OpenAPI for the
// served /openapi.json — never used for route registration, only for the
// document clients read. Each zone's Paths keys are relative (e.g.
// "/companies", since that's required for correct chi route registration
// on Route-mounted subrouters — see the Phase 0 spike finding) and are
// prefixed here with that zone's mount path to produce the true, absolute,
// externally-correct path (e.g. "/api/companies"). Returns an error if two
// zones resolve to the same prefixed path, since that indicates a genuine
// routing conflict rather than something safe to silently overwrite.
func MergeSpecs(title, version string, registry huma.Registry, zones ...ZoneSpec) (*huma.OpenAPI, error) {
	combined := &huma.OpenAPI{
		OpenAPI: "3.1.0",
		Info:    &huma.Info{Title: title, Version: version},
		Components: &huma.Components{
			Schemas: registry,
		},
		Paths: map[string]*huma.PathItem{},
	}
	for _, zone := range zones {
		for relPath, item := range zone.OpenAPI.Paths {
			fullPath := zone.Prefix + relPath
			if _, exists := combined.Paths[fullPath]; exists {
				return nil, fmt.Errorf("apihuma: duplicate path %q across zones (last zone prefix %q)", fullPath, zone.Prefix)
			}
			combined.Paths[fullPath] = item
		}
	}
	return combined, nil
}

// MountCombinedSpec attaches /openapi.json, /openapi.yaml, and /docs routes
// serving the already-merged combined spec onto r. Intended for the ops
// listener (:9090) so documentation never traverses the /api or /admin
// auth/rate-limit middleware. Registers no operations of its own — combined
// must already have every zone's operations merged in via MergeSpecs.
func MountCombinedSpec(r chi.Router, combined *huma.OpenAPI) {
	cfg := huma.Config{
		OpenAPI:      combined,
		OpenAPIPath:  "/openapi",
		DocsPath:     "/docs",
		DocsRenderer: huma.DocsRendererStoplightElements,
	}
	humachi.New(r, cfg)
}
