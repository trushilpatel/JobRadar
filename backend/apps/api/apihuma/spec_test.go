package apihuma

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

type widget struct {
	Name string `json:"name"`
}

type widgetOutput struct {
	Body widget
}

func registerPing(t *testing.T, cfg huma.Config, router chi.Router, opID string) {
	t.Helper()
	api := humachi.New(router, cfg)
	huma.Register(api, huma.Operation{OperationID: opID, Method: http.MethodGet, Path: "/ping"},
		func(ctx context.Context, input *struct{}) (*widgetOutput, error) {
			return &widgetOutput{Body: widget{Name: "pong"}}, nil
		})
}

func TestNewZoneConfig_SharesRegistryAcrossZones(t *testing.T) {
	registry := NewSharedRegistry()
	apiCfg := NewZoneConfig("spike", "0.0.0", registry)
	adminCfg := NewZoneConfig("spike", "0.0.0", registry)

	apiRouter := chi.NewRouter()
	registerPing(t, apiCfg, apiRouter, "ping-api")
	adminRouter := chi.NewRouter()
	registerPing(t, adminCfg, adminRouter, "ping-admin")

	// Both zones registered the same `widget` response type (plus huma's own
	// auto-added default-response schemas, ErrorModel/ErrorDetail) — with a
	// shared registry this must produce exactly one entry per distinct name,
	// never a renamed duplicate like "Widget2" from two independent registries
	// each minting their own copy.
	names := registry.Map()
	if got := len(names); got != 3 {
		t.Fatalf("expected exactly 3 distinct schema entries (Widget, ErrorModel, ErrorDetail) shared across zones, got %d: %v", got, names)
	}
	if _, ok := names["Widget"]; !ok {
		t.Fatalf("expected a single shared \"Widget\" schema entry, got: %v", names)
	}
}

func TestNewZoneConfig_DisablesPerZoneAutoServe(t *testing.T) {
	registry := NewSharedRegistry()
	cfg := NewZoneConfig("spike", "0.0.0", registry)
	if cfg.OpenAPIPath != "" {
		t.Fatalf("expected per-zone OpenAPIPath to be disabled, got %q", cfg.OpenAPIPath)
	}
	if cfg.DocsPath != "" {
		t.Fatalf("expected per-zone DocsPath to be disabled, got %q", cfg.DocsPath)
	}
}

func TestMergeSpecs_PrefixesEachZonesPaths(t *testing.T) {
	registry := NewSharedRegistry()
	apiCfg := NewZoneConfig("jobradar", "1.0.0", registry)
	adminCfg := NewZoneConfig("jobradar", "1.0.0", registry)

	apiRouter := chi.NewRouter()
	registerPing(t, apiCfg, apiRouter, "ping-api")
	adminRouter := chi.NewRouter()
	registerPing(t, adminCfg, adminRouter, "ping-admin")

	combined, err := MergeSpecs("jobradar", "1.0.0", registry,
		ZoneSpec{Prefix: "/api", OpenAPI: apiCfg.OpenAPI},
		ZoneSpec{Prefix: "/admin", OpenAPI: adminCfg.OpenAPI},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := combined.Paths["/api/ping"]; !ok {
		t.Fatalf("expected combined spec to contain /api/ping, got: %v", pathKeys(combined))
	}
	if _, ok := combined.Paths["/admin/ping"]; !ok {
		t.Fatalf("expected combined spec to contain /admin/ping, got: %v", pathKeys(combined))
	}
}

func TestMergeSpecs_ErrorsOnDuplicatePrefixedPath(t *testing.T) {
	registry := NewSharedRegistry()
	zoneACfg := NewZoneConfig("jobradar", "1.0.0", registry)
	zoneBCfg := NewZoneConfig("jobradar", "1.0.0", registry)

	routerA := chi.NewRouter()
	registerPing(t, zoneACfg, routerA, "ping-a")
	routerB := chi.NewRouter()
	registerPing(t, zoneBCfg, routerB, "ping-b")

	// Both zones use prefix "/dup" and both registered "/ping" -> collision.
	_, err := MergeSpecs("jobradar", "1.0.0", registry,
		ZoneSpec{Prefix: "/dup", OpenAPI: zoneACfg.OpenAPI},
		ZoneSpec{Prefix: "/dup", OpenAPI: zoneBCfg.OpenAPI},
	)
	if err == nil {
		t.Fatal("expected an error for duplicate prefixed path /dup/ping, got nil")
	}
}

func TestMountCombinedSpec_ServesOpenAPIJSON(t *testing.T) {
	registry := NewSharedRegistry()
	apiCfg := NewZoneConfig("jobradar", "1.0.0", registry)
	apiRouter := chi.NewRouter()
	registerPing(t, apiCfg, apiRouter, "ping-api")

	combined, err := MergeSpecs("jobradar", "1.0.0", registry, ZoneSpec{Prefix: "/api", OpenAPI: apiCfg.OpenAPI})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	opsRouter := chi.NewRouter()
	MountCombinedSpec(opsRouter, combined)

	srv := httptest.NewServer(opsRouter)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	var doc map[string]any
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatalf("failed to decode served spec: %v", err)
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("expected paths object in served spec, got: %v", doc)
	}
	if _, ok := paths["/api/ping"]; !ok {
		t.Fatalf("expected served spec to contain /api/ping, got: %v", paths)
	}
}

func TestMountCombinedSpec_ServesDocsUI(t *testing.T) {
	registry := NewSharedRegistry()
	combined, err := MergeSpecs("jobradar", "1.0.0", registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	opsRouter := chi.NewRouter()
	MountCombinedSpec(opsRouter, combined)

	srv := httptest.NewServer(opsRouter)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/docs")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/html" {
		t.Fatalf("expected text/html docs page, got %q", ct)
	}
}

func pathKeys(spec *huma.OpenAPI) []string {
	keys := make([]string, 0, len(spec.Paths))
	for k := range spec.Paths {
		keys = append(keys, k)
	}
	return keys
}
