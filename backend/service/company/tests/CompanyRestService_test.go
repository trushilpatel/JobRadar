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
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// fakeValidator is a test double for Validator.
type fakeValidator struct {
	listErr   error
	createErr error
	updateErr error
	activeErr error
}

func (f *fakeValidator) ValidateList(_ companydto.CompanyFilter, _, _ int) error { return f.listErr }
func (f *fakeValidator) ValidateCreate(_ companydto.CreateCompanyInput) error    { return f.createErr }
func (f *fakeValidator) ValidateUpdate(_ companydto.UpdateCompanyInput) error    { return f.updateErr }
func (f *fakeValidator) ValidateSetAllActive(_ *bool) error                      { return f.activeErr }

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
func (f *fakeService) Get(_ context.Context, id string) (*companymodel.Company, error) {
	return f.getResult, f.err
}
func (f *fakeService) Create(_ context.Context, _ companydto.CreateCompanyInput) (*companymodel.Company, error) {
	return f.createResult, f.err
}
func (f *fakeService) Update(_ context.Context, id string, _ companydto.UpdateCompanyInput) error {
	return f.err
}
func (f *fakeService) Delete(_ context.Context, id string) error {
	return f.err
}
func (f *fakeService) SetAllActive(_ context.Context, active bool) error {
	return f.err
}

func newTestRouter(h *companysvc.Handler) *chi.Mux {
	r := chi.NewRouter()
	h.Register(r)
	return r
}

func TestHandlerList_OK(t *testing.T) {
	h := companysvc.NewHandler(&fakeValidator{}, &fakeService{listResult: []companydto.CompanyView{}})
	r := newTestRouter(h)

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
	h := companysvc.NewHandler(
		&fakeValidator{listErr: &domain.ValidationError{Field: "adapter_type", Code: "INVALID_VALUE", Message: "bad"}},
		&fakeService{},
	)
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/companies?adapter_type=bogus", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestHandlerCreate_OK(t *testing.T) {
	company := &companymodel.Company{ID: "stripe", Name: "Stripe"}
	h := companysvc.NewHandler(&fakeValidator{}, &fakeService{createResult: company})
	r := newTestRouter(h)

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
	h := companysvc.NewHandler(
		&fakeValidator{},
		&fakeService{err: domain.ErrInternal.WithMessage("db error")},
	)
	r := newTestRouter(h)

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
	h := companysvc.NewHandler(&fakeValidator{}, &fakeService{})
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/companies/stripe", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestHandlerDelete_NotFound(t *testing.T) {
	h := companysvc.NewHandler(
		&fakeValidator{},
		&fakeService{err: domain.ErrNotFound.WithMessage("company not found")},
	)
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/companies/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestHandlerUpdate_ValidatorError(t *testing.T) {
	h := companysvc.NewHandler(
		&fakeValidator{updateErr: &domain.ValidationError{Field: "", Code: "BAD_REQUEST", Message: "at least one field"}},
		&fakeService{},
	)
	r := newTestRouter(h)

	body, _ := json.Marshal(companydto.UpdateCompanyInput{})
	req := httptest.NewRequest(http.MethodPatch, "/companies/stripe", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestHandlerSetAllActive_OK(t *testing.T) {
	h := companysvc.NewHandler(&fakeValidator{}, &fakeService{})
	r := newTestRouter(h)

	active := true
	body, _ := json.Marshal(map[string]bool{"active": active})
	req := httptest.NewRequest(http.MethodPatch, "/companies/active", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestHandlerSetAllActive_ValidationError(t *testing.T) {
	h := companysvc.NewHandler(
		&fakeValidator{activeErr: &domain.ValidationError{Field: "active", Code: "REQUIRED", Message: "required"}},
		&fakeService{},
	)
	r := newTestRouter(h)

	// Deliberately send invalid body (missing active)
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPatch, "/companies/active", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestHandlerCreate_InvalidBody(t *testing.T) {
	h := companysvc.NewHandler(&fakeValidator{}, &fakeService{})
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/companies", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

// Ensure fakeValidator and fakeService implement their respective interfaces.
var _ companysvc.Validator = (*fakeValidator)(nil)
var _ companysvc.Service = (*fakeService)(nil)

// Suppress unused import warning.
var _ = errors.New
