package scan

import (
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// TestScanRest_ContractAppearsInOpenAPI is the Slice 6 per-slice quality
// gate's contract test.
func TestScanRest_ContractAppearsInOpenAPI(t *testing.T) {
	zoneCfg := apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())
	rest := svc.NewRestService(&fakeScanService{})
	router := svc.NewRestRouter(rest, zoneCfg)
	router.Register(chi.NewRouter())

	cases := []struct {
		path   string
		method string
		opID   string
		status int
	}{
		{"/companies/queue", http.MethodGet, "listScanQueue", 0},
		{"/companies/queue/{entry_id}", http.MethodDelete, "removeFromScanQueue", 0},
		{"/companies/scan-all", http.MethodPost, "triggerScanAll", http.StatusAccepted},
		{"/companies/{id}/scan-stats", http.MethodGet, "getScanStats", 0},
		{"/companies/{id}/scan", http.MethodPost, "triggerScan", http.StatusAccepted},
		{"/companies/{id}/analytics", http.MethodGet, "getCompanyAnalytics", 0},
		{"/companies/{id}/scan-config", http.MethodGet, "getEffectiveScanConfig", 0},
		{"/companies/{id}/scan-interval-override", http.MethodPatch, "updateScanIntervalOverride", 0},
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			item, ok := zoneCfg.Paths[c.path]
			if !ok {
				t.Fatalf("expected %s in the generated spec, got: %v", c.path, zoneCfg.Paths)
			}
			op := operationForMethod(t, item, c.method)
			if op.OperationID != c.opID {
				t.Errorf("OperationID = %q, want %q", op.OperationID, c.opID)
			}
			if c.status != 0 && op.DefaultStatus != c.status {
				t.Errorf("DefaultStatus = %d, want %d", op.DefaultStatus, c.status)
			}
		})
	}
}

func operationForMethod(t *testing.T, item *huma.PathItem, method string) *huma.Operation {
	t.Helper()
	switch method {
	case http.MethodGet:
		return item.Get
	case http.MethodPost:
		return item.Post
	case http.MethodPatch:
		return item.Patch
	case http.MethodDelete:
		return item.Delete
	default:
		t.Fatalf("unsupported method %s in test table", method)
		return nil
	}
}
