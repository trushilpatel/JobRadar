package services_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// TestJobRest_ContractAppearsInOpenAPI is the Slice 2 per-slice quality
// gate's contract test (docs/superpowers/plans/2026-06-13-huma-migration-plan.md,
// Section 4 item 2): all 9 job operations must appear in the generated
// OpenAPI with the correct method, path, and parameters.
func TestJobRest_ContractAppearsInOpenAPI(t *testing.T) {
	zoneCfg := apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())
	rest := jobsvc.NewRestService(&fakeRestSvc{})
	router := jobsvc.NewRestRouter(rest, zoneCfg)
	router.Register(chi.NewRouter())

	cases := []struct {
		path   string
		method string
		opID   string
	}{
		{"/jobs", http.MethodGet, "listJobs"},
		{"/jobs/companies", http.MethodGet, "getJobCompanies"},
		{"/jobs/counts", http.MethodGet, "getJobCounts"},
		{"/jobs/{id}", http.MethodGet, "getJob"},
		{"/jobs/{id}", http.MethodDelete, "dismissJob"},
		{"/jobs/{id}/save", http.MethodPatch, "setJobSaved"},
		{"/jobs/{id}/status", http.MethodPatch, "setJobStatus"},
		{"/jobs/{id}/notes", http.MethodPatch, "setJobNotes"},
		{"/jobs/dismiss/bulk", http.MethodPost, "bulkDismissJobs"},
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

	// Spot-check the richest surface's documented query parameters.
	listOp := zoneCfg.Paths["/jobs"].Get
	for _, name := range []string{"status", "statuses", "companies", "employ_types",
		"experience_ranges", "search", "remote", "sponsorship", "requires_citizen",
		"on_site", "hybrid", "no_requirements", "days_added", "sort_by", "sort_dir",
		"cursor", "limit"} {
		assertHasQueryParam(t, listOp, name)
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

func assertHasQueryParam(t *testing.T, op *huma.Operation, name string) {
	t.Helper()
	for _, p := range op.Parameters {
		if p.Name == name && p.In == "query" {
			return
		}
	}
	t.Errorf("operation %s missing query param %q", op.OperationID, name)
}

// TestJobRest_List_GoldenEnvelope locks the exact byte shape of GET
// /api/jobs, satisfying the envelope-parity gate item.
func TestJobRest_List_GoldenEnvelope(t *testing.T) {
	fake := &fakeRestSvc{listResult: jobdto.PageResult{Items: []jobdto.JobView{}, NextCursor: "nxt"}}
	r := newRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/jobs?cursor=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	want := `{"data":{"items":[],"next_cursor":"nxt"}}` + "\n"
	if w.Body.String() != want {
		t.Fatalf("golden envelope mismatch\ngot:  %q\nwant: %q", w.Body.String(), want)
	}
}

// TestJobRest_Dismiss_MalformedJSONBody_StillDefaultsReason closes an edge
// case that existed in the pre-Huma handler but had no explicit test: a
// malformed (not just missing) body must still silently default the
// dismissal reason, never 400 — see JobRestService.dismissInput.Resolve's
// doc comment for why this requires bypassing Huma's built-in body parsing.
func TestJobRest_Dismiss_MalformedJSONBody_StillDefaultsReason(t *testing.T) {
	fake := &fakeRestSvc{}
	r := newRouter(fake)
	req := httptest.NewRequest(http.MethodDelete, "/jobs/j1", strings.NewReader("{not valid json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", w.Code, w.Body.String())
	}
	if fake.dismissReason != jobcommon.DismissalReasonUserDismissed {
		t.Errorf("reason = %q, want default %q", fake.dismissReason, jobcommon.DismissalReasonUserDismissed)
	}
}
