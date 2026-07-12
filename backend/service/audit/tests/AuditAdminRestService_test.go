package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	auditmodel "github.com/trushilpatel/jobradar/service/audit/models"
	auditsvc "github.com/trushilpatel/jobradar/service/audit/services"
)

// fakeListService is a Service double aimed at REST tests — it captures the
// inbound filter and returns canned (entries, total, err). Only List is
// exercised by the admin REST surface; Append must still be implemented to
// satisfy the contract.
type fakeListService struct {
	gotFilter auditsvc.ListFilter
	entries   []auditmodel.Entry
	total     int64
	err       error
}

func (f *fakeListService) Append(_ context.Context, _ auditmodel.Entry) error { return nil }
func (f *fakeListService) List(_ context.Context, filter auditsvc.ListFilter) ([]auditmodel.Entry, int64, error) {
	f.gotFilter = filter
	return f.entries, f.total, f.err
}

var _ auditsvc.Service = (*fakeListService)(nil)

// newAuditRouter builds a chi router that mounts the admin audit REST under
// /admin so test requests hit the real Router → RestService wiring rather
// than the handler in isolation. Uses a fresh, isolated zone config/registry
// per call — never apispec.APIZoneConfig()/AdminZoneConfig() (see
// apps/api/app_test.go for why that matters).
func newAuditRouter(next auditsvc.Service) *chi.Mux {
	r := chi.NewRouter()
	rest := auditsvc.NewAdminRestService(next)
	router := auditsvc.NewAdminRestRouter(rest, apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry()))
	r.Route("/admin", router.Register)
	return r
}

// pagedShape mirrors handler.RespondPaged so tests can assert the envelope.
type pagedShape struct {
	Data  []auditmodel.Entry `json:"data"`
	Total int                `json:"total"`
	Page  int                `json:"page"`
	Limit int                `json:"limit"`
}

func decodePaged(t *testing.T, body string) pagedShape {
	t.Helper()
	var p pagedShape
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&p); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return p
}

// TestNewAdminRestService_PanicsOnNilNext locks down the construction
// invariant — every REST adapter must have a downstream service.
func TestNewAdminRestService_PanicsOnNilNext(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil next")
		}
	}()
	_ = auditsvc.NewAdminRestService(nil)
}

// TestRestList_OK exercises the happy path: a populated response from the
// service is returned as a 200 paged envelope with the expected counts.
func TestRestList_OK(t *testing.T) {
	t.Parallel()
	svc := &fakeListService{
		entries: []auditmodel.Entry{
			{ID: "row-1", Action: "user.delete"},
			{ID: "row-2", Action: "user.role.grant"},
		},
		total: 2,
	}
	r := newAuditRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/admin/audit-log", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	got := decodePaged(t, w.Body.String())
	if got.Total != 2 || len(got.Data) != 2 {
		t.Errorf("got total=%d len=%d, want 2/2", got.Total, len(got.Data))
	}
	if got.Page != 1 || got.Limit != 20 {
		t.Errorf("got page=%d limit=%d, want 1/20 defaults", got.Page, got.Limit)
	}
}

// TestRestList_NilEntriesNormalisedToEmptyArray asserts the handler converts
// a nil slice from the service into a JSON `[]` not `null` — this keeps the
// response shape predictable for the admin UI.
func TestRestList_NilEntriesNormalisedToEmptyArray(t *testing.T) {
	t.Parallel()
	svc := &fakeListService{entries: nil, total: 0}
	r := newAuditRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/admin/audit-log", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	// Spot-check the JSON: must contain `"data":[]`, never `"data":null`.
	if strings.Contains(body, `"data":null`) {
		t.Errorf("body contains data:null (want []): %s", body)
	}
	if !strings.Contains(body, `"data":[]`) {
		t.Errorf("body missing data:[]: %s", body)
	}
}

// TestRestList_ForwardsFilters asserts every query parameter the handler
// understands ends up on the filter passed downstream. Unknown query
// parameters are ignored.
func TestRestList_ForwardsFilters(t *testing.T) {
	t.Parallel()
	svc := &fakeListService{}
	r := newAuditRouter(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/audit-log?page=3&limit=25&actor_user_id=alice&action=user.delete&ignored=x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if svc.gotFilter.Limit != 25 {
		t.Errorf("Limit = %d, want 25", svc.gotFilter.Limit)
	}
	// Offset = (page-1) * limit = 2 * 25.
	if svc.gotFilter.Offset != 50 {
		t.Errorf("Offset = %d, want 50", svc.gotFilter.Offset)
	}
	if svc.gotFilter.ActorUserID == nil || *svc.gotFilter.ActorUserID != "alice" {
		t.Errorf("ActorUserID = %v, want alice", svc.gotFilter.ActorUserID)
	}
	if svc.gotFilter.Action == nil || *svc.gotFilter.Action != "user.delete" {
		t.Errorf("Action = %v, want user.delete", svc.gotFilter.Action)
	}
}

// TestRestList_OmitsBlankFilters asserts that empty query values leave the
// optional pointer filters nil — important so the SQL WHERE clauses are not
// added when the admin UI clears a filter.
func TestRestList_OmitsBlankFilters(t *testing.T) {
	t.Parallel()
	svc := &fakeListService{}
	r := newAuditRouter(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/audit-log?actor_user_id=&action=", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if svc.gotFilter.ActorUserID != nil {
		t.Errorf("ActorUserID = %v, want nil", svc.gotFilter.ActorUserID)
	}
	if svc.gotFilter.Action != nil {
		t.Errorf("Action = %v, want nil", svc.gotFilter.Action)
	}
}

// TestRestList_DefaultPaginationWhenAbsent asserts the request's lack of
// page/limit query params results in page=1, limit=20, offset=0.
func TestRestList_DefaultPaginationWhenAbsent(t *testing.T) {
	t.Parallel()
	svc := &fakeListService{}
	r := newAuditRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/admin/audit-log", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if svc.gotFilter.Limit != 20 {
		t.Errorf("Limit = %d, want 20", svc.gotFilter.Limit)
	}
	if svc.gotFilter.Offset != 0 {
		t.Errorf("Offset = %d, want 0", svc.gotFilter.Offset)
	}
}

// TestRestList_PropagatesServiceError asserts that a ServiceError from the
// next layer is mapped to its HTTP status via handler.HandleError — not
// swallowed, not 200'd with an empty body.
func TestRestList_PropagatesServiceError(t *testing.T) {
	t.Parallel()
	svc := &fakeListService{err: apperr.ErrInternal.WithMessage("db down")}
	r := newAuditRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/admin/audit-log", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	// Internal detail must never leak to the response body — the public
	// message is the generic "internal server error" string.
	if strings.Contains(w.Body.String(), "db down") {
		t.Errorf("internal message leaked into body: %s", w.Body.String())
	}
}

// TestRestList_PropagatesGenericError verifies that a non-ServiceError still
// produces a 500 (HandleError treats unknown errors as INTERNAL).
func TestRestList_PropagatesGenericError(t *testing.T) {
	t.Parallel()
	svc := &fakeListService{err: errors.New("plain")}
	r := newAuditRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/admin/audit-log", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// TestRestList_OnlyGetRegistered asserts that POST /admin/audit-log is not
// mounted — the router exposes a single GET handler so future regressions
// that add a write route accidentally are surfaced.
func TestRestList_OnlyGetRegistered(t *testing.T) {
	t.Parallel()
	svc := &fakeListService{}
	r := newAuditRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/admin/audit-log", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 405 or 404 (no POST route mounted)", w.Code)
	}
}

// TestNewAdminRestRouter_Construction is a minimal smoke check that the
// router constructor returns a non-nil value — guards against an accidental
// nil-return refactor.
func TestNewAdminRestRouter_Construction(t *testing.T) {
	t.Parallel()
	h := auditsvc.NewAdminRestService(&fakeListService{})
	if got := auditsvc.NewAdminRestRouter(h, apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())); got == nil {
		t.Fatal("NewAdminRestRouter returned nil")
	}
}
