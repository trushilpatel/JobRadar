package company_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

func newAdminRouter(svc companysvc.CompanyAdminService) *chi.Mux {
	rest := companysvc.NewAdminRestService(svc)
	router := companysvc.NewAdminRestRouter(rest)
	r := chi.NewRouter()
	router.Register(r)
	return r
}

func TestAdminRest_Create_PublicOnly(t *testing.T) {
	svc := companysvc.NewAdminValidationService(&fakeAdmin{
		createView: &companydto.CompanyAdminView{CompanyAdminCommon: companydto.CompanyAdminCommon{Company: companymodel.Company{ID: "stripe", Name: "Stripe", Slug: "stripe"}}},
	})
	r := newAdminRouter(svc)

	body, _ := json.Marshal(map[string]any{
		"id":   "stripe",
		"name": "Stripe",
		"slug": "stripe",
	})
	req := httptest.NewRequest(http.MethodPost, "/companies", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
}

func TestAdminRest_Create_WithSettings(t *testing.T) {
	svc := companysvc.NewAdminValidationService(&fakeAdmin{
		createView: &companydto.CompanyAdminView{CompanyAdminCommon: companydto.CompanyAdminCommon{Company: companymodel.Company{ID: "stripe", Name: "Stripe", Slug: "stripe"}}},
	})
	r := newAdminRouter(svc)

	body, _ := json.Marshal(map[string]any{
		"id":   "stripe",
		"name": "Stripe",
		"slug": "stripe",
		"scan_settings": map[string]any{
			"adapter_type": companycommon.AdapterGreenhouse,
			"active":       true,
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/companies", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
}

func TestAdminRest_Create_ValidationError(t *testing.T) {
	svc := companysvc.NewAdminValidationService(&fakeAdmin{})
	r := newAdminRouter(svc)

	body, _ := json.Marshal(map[string]any{}) // missing id/name/slug
	req := httptest.NewRequest(http.MethodPost, "/companies", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAdminRest_UpdatePublic_OK(t *testing.T) {
	svc := companysvc.NewAdminValidationService(&fakeAdmin{})
	r := newAdminRouter(svc)

	body, _ := json.Marshal(companydto.CompanyPublicInput{Name: "Renamed"})
	req := httptest.NewRequest(http.MethodPatch, "/companies/stripe", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestAdminRest_UpdateScanSettings_OK(t *testing.T) {
	svc := companysvc.NewAdminValidationService(&fakeAdmin{})
	r := newAdminRouter(svc)

	body, _ := json.Marshal(companydto.CompanyScanSettingsInput{Active: boolPtr(false)})
	req := httptest.NewRequest(http.MethodPatch, "/companies/stripe/scan-settings", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestAdminRest_Delete_NotFound(t *testing.T) {
	svc := &fakeAdmin{deleteErr: apperr.ErrNotFound.WithMessage("company not found")}
	r := newAdminRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/companies/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestAdminRest_SetAllActive_OK(t *testing.T) {
	r := newAdminRouter(&fakeAdmin{})

	body, _ := json.Marshal(map[string]bool{"active": true})
	req := httptest.NewRequest(http.MethodPatch, "/companies/active", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAdminRest_SetAllActive_MissingActive(t *testing.T) {
	r := newAdminRouter(&fakeAdmin{})

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPatch, "/companies/active", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
