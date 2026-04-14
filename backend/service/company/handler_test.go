package company

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/api/response"
	"github.com/trushilpatel/jobradar/model"
)

// fakeValidator is a test double for Validator.
type fakeValidator struct {
	listErr    error
	createErr  error
	updateErr  error
	activeErr  error
}

func (f *fakeValidator) ValidateList(_ CompanyFilter) error         { return f.listErr }
func (f *fakeValidator) ValidateCreate(_ CreateCompanyInput) error  { return f.createErr }
func (f *fakeValidator) ValidateUpdate(_ UpdateCompanyInput) error  { return f.updateErr }
func (f *fakeValidator) ValidateSetAllActive(_ *bool) error        { return f.activeErr }

// fakeService is a test double for Service.
type fakeService struct {
	listResult   []CompanyView
	getResult    *model.Company
	createResult *model.Company
	err          error
}

func (f *fakeService) List(_ CompanyFilter) ([]CompanyView, error) {
	return f.listResult, f.err
}
func (f *fakeService) Get(id string) (*model.Company, error) {
	return f.getResult, f.err
}
func (f *fakeService) Create(_ CreateCompanyInput) (*model.Company, error) {
	return f.createResult, f.err
}
func (f *fakeService) Update(id string, _ UpdateCompanyInput) error {
	return f.err
}
func (f *fakeService) Delete(id string) error {
	return f.err
}
func (f *fakeService) SetAllActive(active bool) error {
	return f.err
}

func newTestRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	h.Register(r)
	return r
}

func TestHandlerList_OK(t *testing.T) {
	h := NewHandler(&fakeValidator{}, &fakeService{listResult: []CompanyView{}})
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
	h := NewHandler(
		&fakeValidator{listErr: &response.ValidationError{Field: "adapter_type", Code: "INVALID_VALUE", Message: "bad"}},
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
	company := &model.Company{ID: "stripe", Name: "Stripe"}
	h := NewHandler(&fakeValidator{}, &fakeService{createResult: company})
	r := newTestRouter(h)

	body, _ := json.Marshal(CreateCompanyInput{
		ID: "stripe", Name: "Stripe", URL: "https://example.com", AdapterType: model.AdapterGreenhouse,
	})
	req := httptest.NewRequest(http.MethodPost, "/companies", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("got status %d, want 201", w.Code)
	}
}

func TestHandlerCreate_ServiceError(t *testing.T) {
	h := NewHandler(
		&fakeValidator{},
		&fakeService{err: response.ErrInternal.WithMessage("db error")},
	)
	r := newTestRouter(h)

	body, _ := json.Marshal(CreateCompanyInput{
		ID: "stripe", Name: "Stripe", URL: "https://example.com", AdapterType: model.AdapterGreenhouse,
	})
	req := httptest.NewRequest(http.MethodPost, "/companies", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestHandlerDelete_OK(t *testing.T) {
	h := NewHandler(&fakeValidator{}, &fakeService{})
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/companies/stripe", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestHandlerDelete_NotFound(t *testing.T) {
	h := NewHandler(
		&fakeValidator{},
		&fakeService{err: response.ErrNotFound.WithMessage("company not found")},
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
	h := NewHandler(
		&fakeValidator{updateErr: &response.ValidationError{Field: "", Code: "BAD_REQUEST", Message: "at least one field"}},
		&fakeService{},
	)
	r := newTestRouter(h)

	body, _ := json.Marshal(UpdateCompanyInput{})
	req := httptest.NewRequest(http.MethodPatch, "/companies/stripe", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestHandlerSetAllActive_OK(t *testing.T) {
	h := NewHandler(&fakeValidator{}, &fakeService{})
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
	h := NewHandler(
		&fakeValidator{activeErr: &response.ValidationError{Field: "active", Code: "REQUIRED", Message: "required"}},
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
	h := NewHandler(&fakeValidator{}, &fakeService{})
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/companies", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

// Ensure fakeValidator and fakeService implement their respective interfaces.
var _ Validator = (*fakeValidator)(nil)
var _ Service = (*fakeService)(nil)

// Suppress unused import warning.
var _ = errors.New
