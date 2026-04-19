package company_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/common/domain"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// fakeService is a test double for Service.
type fakeService struct {
	listResult   []companydto.CompanyView
	getResult    *companymodel.Company
	createResult *companymodel.Company
	err          error
}

func (f *fakeService) List(_ context.Context, _ companydto.CompanyFilter, _, _ int) (companydto.CompanyListResult, error) {
	return companydto.CompanyListResult{Items: f.listResult, Total: int64(len(f.listResult))}, f.err
}
func (f *fakeService) GetAll(_ context.Context) ([]companydto.CompanyView, error) {
	return f.listResult, f.err
}
func (f *fakeService) Get(_ context.Context, _ string) (*companymodel.Company, error) {
	return f.getResult, f.err
}
func (f *fakeService) Create(_ context.Context, _ companydto.CreateCompanyInput) (*companymodel.Company, error) {
	return f.createResult, f.err
}
func (f *fakeService) Update(_ context.Context, _ string, _ companydto.UpdateCompanyInput) error {
	return f.err
}
func (f *fakeService) Delete(_ context.Context, _ string) error     { return f.err }
func (f *fakeService) SetAllActive(_ context.Context, _ bool) error { return f.err }

// newTestRouter builds a chi router with the full chain:
// validationService → fakeService, mounted via RestRouter.
func newTestRouter(svc companysvc.Service) *chi.Mux {
	rest := companysvc.NewRestService(svc)
	router := companysvc.NewRestRouter(rest)
	r := chi.NewRouter()
	router.Register(r)
	return r
}

func TestHandlerList_OK(t *testing.T) {
	r := newTestRouter(&fakeService{listResult: []companydto.CompanyView{}})

	req := httptest.NewRequest(http.MethodGet, "/companies", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"data"`) {
		t.Errorf("response missing data envelope: %s", w.Body.String())
	}
}

func TestHandlerList_ValidatorError(t *testing.T) {
	// Validation is in the chain — use a real validationService wrapping fakeService.
	svc := companysvc.NewValidator(&fakeService{})
	r := newTestRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/companies?adapter_type=bogus", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestHandlerCreate_OK(t *testing.T) {
	company := &companymodel.Company{ID: "stripe", Name: "Stripe"}
	r := newTestRouter(&fakeService{createResult: company})

	body, _ := json.Marshal(companydto.CreateCompanyInput{
		ID: "stripe", Name: "Stripe", URL: "https://example.com", AdapterType: companycommon.AdapterGreenhouse,
	})
	req := httptest.NewRequest(http.MethodPost, "/companies", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("got status %d, want 201", w.Code)
	}
}

func TestHandlerCreate_ServiceError(t *testing.T) {
	r := newTestRouter(&fakeService{err: domain.ErrInternal.WithMessage("db error")})

	body, _ := json.Marshal(companydto.CreateCompanyInput{
		ID: "stripe", Name: "Stripe", URL: "https://example.com", AdapterType: companycommon.AdapterGreenhouse,
	})
	req := httptest.NewRequest(http.MethodPost, "/companies", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestHandlerDelete_OK(t *testing.T) {
	r := newTestRouter(&fakeService{})

	req := httptest.NewRequest(http.MethodDelete, "/companies/stripe", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestHandlerDelete_NotFound(t *testing.T) {
	r := newTestRouter(&fakeService{err: domain.ErrNotFound.WithMessage("company not found")})

	req := httptest.NewRequest(http.MethodDelete, "/companies/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestHandlerUpdate_ValidatorError(t *testing.T) {
	// No fields — validation layer should reject it.
	svc := companysvc.NewValidator(&fakeService{})
	r := newTestRouter(svc)

	body, _ := json.Marshal(companydto.UpdateCompanyInput{})
	req := httptest.NewRequest(http.MethodPatch, "/companies/stripe", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestHandlerSetAllActive_OK(t *testing.T) {
	r := newTestRouter(&fakeService{})

	body, _ := json.Marshal(map[string]bool{"active": true})
	req := httptest.NewRequest(http.MethodPatch, "/companies/active", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestHandlerSetAllActive_MissingActive(t *testing.T) {
	r := newTestRouter(&fakeService{})

	// Body missing the active field — RestService guards this directly.
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPatch, "/companies/active", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestHandlerCreate_InvalidBody(t *testing.T) {
	r := newTestRouter(&fakeService{})

	req := httptest.NewRequest(http.MethodPost, "/companies", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

// Ensure fakeService implements Service.
var _ companysvc.Service = (*fakeService)(nil)

// Suppress unused import warning.
var _ = errors.New
