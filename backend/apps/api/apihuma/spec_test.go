package apihuma

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// TestNewZoneConfig_ConcurrentCallsAreRaceFree guards a real finding from
// Slice 7: NewZoneConfig used to call InstallLegacyErrorModel() on every
// invocation, an unsynchronized write to huma's process-global NewError var.
// Any test package using t.Parallel() (many do) that builds a zone config
// per-test triggers a genuine data race under `go test -race` — first
// caught in service/user/tests. NewZoneConfig must be safe to call
// concurrently from many goroutines.
func TestNewZoneConfig_ConcurrentCallsAreRaceFree(t *testing.T) {
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			NewZoneConfig("test", "0.0.0", NewSharedRegistry())
		})
	}
	wg.Wait()
}

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

// TestNewZoneConfig_InstallsLegacyErrorModel guards against a real gap found
// while writing Slice 3's tests: huma.NewError is a process-global var, and
// production only gets the legacy error shape because main()/printSpec()
// explicitly call InstallLegacyErrorModel() before any registration. A
// slice's own unit tests, run as their own process via `go test`, never hit
// that call path — a request that fails Huma's OWN body/param validation
// (e.g. malformed JSON, before the handler ever runs) would silently get
// Huma's default RFC 9457 shape instead of the legacy one, even though the
// real server never does. NewZoneConfig must install it itself (at least
// once per process, see legacyErrorModelOnce) so every caller — production
// or test — gets consistent behavior regardless of whether they remembered
// the explicit call.
//
// legacyErrorModelOnce is reset here to simulate NewZoneConfig's very first
// call in a process — the guarantee this test protects only holds for that
// first call (see the sync.Once fix for the Slice 7 data race); every other
// test in this package process has almost certainly already consumed the
// real Once by the time this test runs.
func TestNewZoneConfig_InstallsLegacyErrorModel(t *testing.T) {
	original := huma.NewError
	defer func() { huma.NewError = original }()
	huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
		return &huma.ErrorModel{Status: status, Detail: msg}
	}

	legacyErrorModelOnce = sync.Once{}
	NewZoneConfig("spike", "0.0.0", NewSharedRegistry())

	err := huma.NewError(http.StatusBadRequest, "boom")
	got, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("marshal failed: %v", marshalErr)
	}
	if !strings.Contains(string(got), `"error"`) {
		t.Fatalf("expected NewZoneConfig to install the legacy error model, got default-shaped error: %s", got)
	}
}

// TestNewZoneConfig_NoSchemaLinkInjection guards byte-for-byte envelope
// parity (3.1): huma.DefaultConfig's own CreateHook injects a "$schema"
// field and Link headers into every JSON response by default (a discoverability
// feature for editors) — that is an uninvited extra field the legacy frontend
// has never seen. NewZoneConfig must disable it.
func TestNewZoneConfig_NoSchemaLinkInjection(t *testing.T) {
	registry := NewSharedRegistry()
	cfg := NewZoneConfig("spike", "0.0.0", registry)
	router := chi.NewRouter()
	registerPing(t, cfg, router, "ping-noschema")

	srv := httptest.NewServer(router)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/ping")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if got := res.Header.Get("Link"); got != "" {
		t.Fatalf("expected no Link header (schema-link transformer must be disabled), got %q", got)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if strings.Contains(string(body), "$schema") {
		t.Fatalf("expected no $schema field in response body (breaks byte parity with the legacy envelope), got: %s", body)
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
