package company_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// TestPublicRest_ContractAppearsInOpenAPI is the Slice 1 per-slice quality
// gate's contract test (docs/superpowers/plans/2026-06-13-huma-migration-plan.md,
// Section 4 item 2): the migrated operations must appear in the generated
// OpenAPI with the correct method, path, and parameters.
func TestPublicRest_ContractAppearsInOpenAPI(t *testing.T) {
	zoneCfg := apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())
	rest := companysvc.NewRestService(&fakePublic{})
	router := companysvc.NewRestRouter(rest, zoneCfg)
	router.Register(chi.NewRouter())

	listOp, ok := zoneCfg.Paths["/companies"]
	if !ok || listOp.Get == nil {
		t.Fatalf("expected GET /companies in the generated spec, got: %v", zoneCfg.Paths)
	}
	if listOp.Get.OperationID != "listCompaniesPublic" {
		t.Errorf("OperationID = %q, want listCompaniesPublic", listOp.Get.OperationID)
	}
	assertHasQueryParam(t, listOp.Get, "search")
	assertHasQueryParam(t, listOp.Get, "sort_by")
	assertHasQueryParam(t, listOp.Get, "sort_dir")
	assertHasQueryParam(t, listOp.Get, "cursor")
	assertHasQueryParam(t, listOp.Get, "limit")

	getOp, ok := zoneCfg.Paths["/companies/{id}"]
	if !ok || getOp.Get == nil {
		t.Fatalf("expected GET /companies/{id} in the generated spec, got: %v", zoneCfg.Paths)
	}
	if getOp.Get.OperationID != "getCompanyPublic" {
		t.Errorf("OperationID = %q, want getCompanyPublic", getOp.Get.OperationID)
	}
	assertHasPathParam(t, getOp.Get, "id")
}

func assertHasQueryParam(t *testing.T, op *huma.Operation, name string) {
	t.Helper()
	for _, p := range op.Parameters {
		if p.Name == name && p.In == "query" {
			return
		}
	}
	t.Errorf("operation %s missing query param %q, got: %+v", op.OperationID, name, op.Parameters)
}

func assertHasPathParam(t *testing.T, op *huma.Operation, name string) {
	t.Helper()
	for _, p := range op.Parameters {
		if p.Name == name && p.In == "path" {
			return
		}
	}
	t.Errorf("operation %s missing path param %q, got: %+v", op.OperationID, name, op.Parameters)
}

// TestPublicRest_List_GoldenEnvelope locks the exact byte shape of
// GET /api/companies, satisfying the envelope-parity gate item independent
// of apihuma's generic (type-only) golden tests.
func TestPublicRest_List_GoldenEnvelope(t *testing.T) {
	cursor := "next-abc"
	svc := &fakePublic{listResult: companydto.CompanyPublicListResult{
		Items:      []companydto.CompanyPublicView{{Company: companymodel.Company{ID: "stripe", Name: "Stripe", Slug: "stripe"}}},
		Total:      1,
		NextCursor: &cursor,
	}}
	r := newPublicRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/companies?limit=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	// Trailing \n matches json.Encoder's default behavior — both the legacy
	// handler.Respond and Huma's writer use it, so this is exact parity, not
	// a Huma-introduced difference.
	want := "{\"data\":[{\"id\":\"stripe\",\"name\":\"Stripe\",\"slug\":\"stripe\",\"created_at\":\"0001-01-01T00:00:00Z\",\"updated_at\":\"0001-01-01T00:00:00Z\"}],\"total\":1,\"limit\":1,\"next_cursor\":\"next-abc\"}\n"
	if w.Body.String() != want {
		t.Fatalf("golden envelope mismatch\ngot:  %q\nwant: %q", w.Body.String(), want)
	}
}
