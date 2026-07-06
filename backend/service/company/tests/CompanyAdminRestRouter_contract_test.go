package company_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// TestAdminRest_ContractAppearsInOpenAPI is Slice 10's per-slice quality
// gate contract test. Also guards the literal-vs-{id} ordering
// (/companies/active) — both paths must appear distinctly, /active must
// never be swallowed into /{id}.
func TestAdminRest_ContractAppearsInOpenAPI(t *testing.T) {
	zoneCfg := apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())
	rest := companysvc.NewAdminRestService(&fakeAdmin{})
	router := companysvc.NewAdminRestRouter(rest, zoneCfg)
	router.Register(chi.NewRouter())

	listOp, ok := zoneCfg.Paths["/companies"]
	if !ok || listOp.Get == nil || listOp.Post == nil {
		t.Fatalf("expected GET+POST /companies in the generated spec, got: %v", zoneCfg.Paths)
	}
	if listOp.Get.OperationID != "listCompaniesAdmin" {
		t.Errorf("list OperationID = %q, want listCompaniesAdmin", listOp.Get.OperationID)
	}
	if listOp.Post.OperationID != "createCompanyAdmin" {
		t.Errorf("create OperationID = %q, want createCompanyAdmin", listOp.Post.OperationID)
	}
	if listOp.Post.DefaultStatus != http.StatusCreated {
		t.Errorf("create DefaultStatus = %d, want 201", listOp.Post.DefaultStatus)
	}

	activeOp, ok := zoneCfg.Paths["/companies/active"]
	if !ok || activeOp.Patch == nil {
		t.Fatalf("expected PATCH /companies/active in the generated spec, got: %v", zoneCfg.Paths)
	}
	if activeOp.Patch.OperationID != "setAllCompaniesActive" {
		t.Errorf("setAllActive OperationID = %q, want setAllCompaniesActive", activeOp.Patch.OperationID)
	}

	idOp, ok := zoneCfg.Paths["/companies/{id}"]
	if !ok || idOp.Get == nil || idOp.Patch == nil || idOp.Delete == nil {
		t.Fatalf("expected GET+PATCH+DELETE /companies/{id} in the generated spec, got: %v", zoneCfg.Paths)
	}
	if idOp.Get.OperationID != "getCompanyAdmin" {
		t.Errorf("get OperationID = %q, want getCompanyAdmin", idOp.Get.OperationID)
	}
	if idOp.Patch.OperationID != "updateCompanyPublic" {
		t.Errorf("update OperationID = %q, want updateCompanyPublic", idOp.Patch.OperationID)
	}
	if idOp.Delete.OperationID != "deleteCompanyAdmin" {
		t.Errorf("delete OperationID = %q, want deleteCompanyAdmin", idOp.Delete.OperationID)
	}

	scanSettingsOp, ok := zoneCfg.Paths["/companies/{id}/scan-settings"]
	if !ok || scanSettingsOp.Patch == nil {
		t.Fatalf("expected PATCH /companies/{id}/scan-settings in the generated spec, got: %v", zoneCfg.Paths)
	}
	if scanSettingsOp.Patch.OperationID != "updateCompanyScanSettings" {
		t.Errorf("scan-settings OperationID = %q, want updateCompanyScanSettings", scanSettingsOp.Patch.OperationID)
	}
}
