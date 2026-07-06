package apispec

import (
	"context"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
)

func TestBuild_NoZonesProducesEmptyPaths(t *testing.T) {
	// Documents the pre-Slice-1 baseline: calling Build with no zones is
	// still valid and produces an empty-but-well-formed document.
	spec, err := Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec == nil {
		t.Fatal("expected a non-nil spec")
	}
	if spec.Info == nil || spec.Info.Version != Version {
		t.Fatalf("expected spec.Info.Version %q, got %+v", Version, spec.Info)
	}
	if len(spec.Paths) != 0 {
		t.Fatalf("expected zero paths with no zones passed, got %d: %v", len(spec.Paths), spec.Paths)
	}
}

func TestBuild_MergesGivenZones(t *testing.T) {
	zoneCfg := APIZoneConfig()
	api := humachi.New(chi.NewRouter(), zoneCfg)
	huma.Register(api, huma.Operation{OperationID: "ping", Method: http.MethodGet, Path: "/companies"},
		func(ctx context.Context, input *struct{}) (*struct{ Body string }, error) {
			return &struct{ Body string }{Body: "ok"}, nil
		})

	spec, err := Build(apihuma.ZoneSpec{Prefix: "/api", OpenAPI: zoneCfg.OpenAPI})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := spec.Paths["/api/companies"]; !ok {
		t.Fatalf("expected /api/companies in merged spec, got: %v", spec.Paths)
	}
}

func TestAPIZoneConfig_UsesSharedRegistryAndDisablesAutoServe(t *testing.T) {
	cfg := APIZoneConfig()
	if cfg.Components == nil || cfg.Components.Schemas == nil {
		t.Fatal("expected APIZoneConfig to have a schema registry")
	}
	if cfg.OpenAPIPath != "" || cfg.DocsPath != "" {
		t.Fatalf("expected per-zone auto-serve disabled, got OpenAPIPath=%q DocsPath=%q", cfg.OpenAPIPath, cfg.DocsPath)
	}
}

func TestAPIZoneConfig_EachCallIsIndependent(t *testing.T) {
	// Callers must build the zone config exactly ONCE and thread the same
	// value through both router wiring and the later Build() call — two
	// independent calls must not secretly resolve to the same *huma.OpenAPI,
	// or an accidental second call would silently merge into the wrong doc.
	a := APIZoneConfig()
	b := APIZoneConfig()
	if a.OpenAPI == b.OpenAPI {
		t.Fatal("expected two APIZoneConfig() calls to produce independent *huma.OpenAPI instances")
	}
}
