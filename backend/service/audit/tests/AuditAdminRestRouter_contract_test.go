package tests

import (
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	auditsvc "github.com/trushilpatel/jobradar/service/audit/services"
)

// TestAuditRest_ContractAppearsInOpenAPI is Slice 12's per-slice quality
// gate contract test — the final slice in the primary migration sequence.
func TestAuditRest_ContractAppearsInOpenAPI(t *testing.T) {
	zoneCfg := apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())
	rest := auditsvc.NewAdminRestService(&fakeListService{})
	router := auditsvc.NewAdminRestRouter(rest, zoneCfg)
	router.Register(chi.NewRouter())

	item, ok := zoneCfg.Paths["/audit-log"]
	if !ok || item.Get == nil {
		t.Fatalf("expected GET /audit-log in the generated spec, got: %v", zoneCfg.Paths)
	}
	if item.Get.OperationID != "listAuditLog" {
		t.Errorf("OperationID = %q, want listAuditLog", item.Get.OperationID)
	}
	if item.Post != nil {
		t.Error("expected no POST /audit-log operation registered")
	}
}
