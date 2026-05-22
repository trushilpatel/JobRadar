package company_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// Compile-time assertion that recordingPublicForRest satisfies CompanyService.
var _ companysvc.CompanyService = (*recordingPublicForRest)(nil)

// TestPublicRest_List_QueryParsing checks that the public list endpoint pulls
// the canonical query params onto CompanyFilter and forwards them.
func TestPublicRest_List_QueryParsing(t *testing.T) {
	rec := &recordingPublicForRest{}
	r := newPublicRouter(rec)

	req := httptest.NewRequest(http.MethodGet, "/companies?search=stripe&sort_by=created_at&sort_dir=desc&limit=15", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if rec.lastFilter.Search != "stripe" || rec.lastFilter.SortBy != "created_at" || rec.lastFilter.SortDir != "desc" {
		t.Errorf("filter not parsed: %+v", rec.lastFilter)
	}
	if rec.lastLimit != 15 {
		t.Errorf("limit=%d, want 15", rec.lastLimit)
	}
}

// TestPublicRest_List_ServiceError surfaces an upstream INTERNAL as a 500.
func TestPublicRest_List_ServiceError(t *testing.T) {
	rec := &recordingPublicForRest{listErr: apperr.ErrInternal.WithMessage("boom")}
	r := newPublicRouter(rec)
	req := httptest.NewRequest(http.MethodGet, "/companies", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// TestPublicRest_Get_NotFound asserts the 404 mapping for a NOT_FOUND
// ServiceError from the public chain.
func TestPublicRest_Get_NotFound(t *testing.T) {
	rec := &recordingPublicForRest{getErr: apperr.ErrNotFound.WithMessage("missing")}
	r := newPublicRouter(rec)
	req := httptest.NewRequest(http.MethodGet, "/companies/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// recordingPublicForRest records the inputs the rest layer forwards and lets
// us pin canned errors.
type recordingPublicForRest struct {
	lastFilter companydto.CompanyFilter
	lastLimit  int
	listErr    error
	getErr     error
}

func (r *recordingPublicForRest) ListPublic(_ context.Context, f companydto.CompanyFilter, limit int) (companydto.CompanyPublicListResult, error) {
	r.lastFilter = f
	r.lastLimit = limit
	return companydto.CompanyPublicListResult{}, r.listErr
}

func (r *recordingPublicForRest) GetPublic(_ context.Context, _ string) (*companydto.CompanyPublicView, error) {
	return nil, r.getErr
}
