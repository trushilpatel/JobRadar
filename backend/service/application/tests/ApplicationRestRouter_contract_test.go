package application

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	svc "github.com/trushilpatel/jobradar/service/application/services"
)

// TestApplicationRest_ContractAppearsInOpenAPI is the Slice 3 per-slice
// quality gate's contract test (docs/superpowers/plans/2026-06-13-huma-migration-plan.md,
// Section 4 item 2): all 6 application operations must appear in the
// generated OpenAPI with the correct method and path.
func TestApplicationRest_ContractAppearsInOpenAPI(t *testing.T) {
	zoneCfg := newTestZoneConfig()
	rest := svc.NewRestService(&fakeService{})
	router := svc.NewRestRouter(rest, zoneCfg)
	router.Register(chi.NewRouter())

	cases := []struct {
		path   string
		method string
		opID   string
	}{
		{"/jobs/{id}/applications", http.MethodGet, "listApplicationsByJob"},
		{"/jobs/{id}/applications", http.MethodPost, "createApplication"},
		{"/jobs/{id}/applications/{app_id}/stage", http.MethodPatch, "updateApplicationStage"},
		{"/jobs/{id}/applications/{app_id}/note", http.MethodPatch, "updateApplicationNote"},
		{"/applications", http.MethodGet, "listAllApplications"},
		{"/applications/{id}/stage", http.MethodPatch, "updateApplicationStageByID"},
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
		})
	}

	createOp := zoneCfg.Paths["/jobs/{id}/applications"].Post
	if createOp.DefaultStatus != http.StatusCreated {
		t.Errorf("create DefaultStatus = %d, want 201", createOp.DefaultStatus)
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

// TestApplicationRest_UpdateStage_GoldenEnvelope locks the exact byte shape
// of PATCH /api/jobs/{id}/applications/{app_id}/stage, satisfying the
// envelope-parity gate item.
func TestApplicationRest_UpdateStage_GoldenEnvelope(t *testing.T) {
	r := newTestRouter(&fakeService{})

	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/77/stage", strings.NewReader(`{"stage":"offer"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	want := "{\"data\":{\"app_id\":\"77\",\"stage\":\"offer\"}}\n"
	if w.Body.String() != want {
		t.Fatalf("golden envelope mismatch\ngot:  %q\nwant: %q", w.Body.String(), want)
	}
}

// TestApplicationRest_Create_Returns201 confirms the explicit DefaultStatus
// override actually takes effect at the HTTP layer, not just in the
// documented spec.
func TestApplicationRest_Create_Returns201(t *testing.T) {
	app := &appmodel.JobApplication{ID: 7, JobResultID: "j1", Stage: "applied"}
	r := newTestRouter(&fakeService{app: app})

	req := httptest.NewRequest(http.MethodPost, "/jobs/j1/applications", strings.NewReader(`{"stage":"applied"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
}
