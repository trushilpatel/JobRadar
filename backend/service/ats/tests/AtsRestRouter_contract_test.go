package tests

import (
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
)

// TestAtsRest_ContractAppearsInOpenAPI is Slice 8's per-slice quality gate
// contract test. Ats is the first /admin-zone slice migrated — it registers
// relative paths ("/ats", "/ats/{name}") directly on whatever router it's
// given, exactly like every /api-zone slice; the "/admin" prefix is added
// only by the caller's mount point (adminRouteGroup.Register) and by
// apispec.Build's zone-prefix merge, never by this router itself.
func TestAtsRest_ContractAppearsInOpenAPI(t *testing.T) {
	zoneCfg := apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())
	rest := atssvc.NewRestService(newStubAtsService())
	router := atssvc.NewRestRouter(rest, zoneCfg)
	router.Register(chi.NewRouter())

	listItem, ok := zoneCfg.Paths["/ats"]
	if !ok || listItem.Get == nil {
		t.Fatalf("expected GET /ats in the generated spec, got: %v", zoneCfg.Paths)
	}
	if listItem.Get.OperationID != "listAts" {
		t.Errorf("list OperationID = %q, want listAts", listItem.Get.OperationID)
	}

	itemPath, ok := zoneCfg.Paths["/ats/{name}"]
	if !ok || itemPath.Get == nil || itemPath.Patch == nil {
		t.Fatalf("expected GET+PATCH /ats/{name} in the generated spec, got: %v", zoneCfg.Paths)
	}
	if itemPath.Get.OperationID != "getAts" {
		t.Errorf("get OperationID = %q, want getAts", itemPath.Get.OperationID)
	}
	if itemPath.Patch.OperationID != "updateAts" {
		t.Errorf("patch OperationID = %q, want updateAts", itemPath.Patch.OperationID)
	}
}
