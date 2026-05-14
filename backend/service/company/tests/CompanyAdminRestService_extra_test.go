package company_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// Compile-time assertion: recordingAdminForRest must satisfy the admin
// service interface so it can be passed to NewAdminRestService.
var _ companysvc.CompanyAdminService = (*recordingAdminForRest)(nil)

// recordingAdminForRest is a CompanyAdminService that records inputs so we
// can verify the REST layer parses query / body / path correctly.
type recordingAdminForRest struct {
	listFilter      companydto.CompanyFilter
	listPage        int
	listLimit       int
	listResult      companydto.CompanyAdminListResult
	listErr         error
	getID           string
	getView         *companydto.CompanyAdminView
	getErr          error
	createPublic    companydto.CompanyPublicInput
	createSettings  companydto.CompanyScanSettingsInput
	createView      *companydto.CompanyAdminView
	createErr       error
	updPublicID     string
	updPublicInput  companydto.CompanyPublicInput
	updPublicErr    error
	updSettingsID   string
	updSettingsIn   companydto.CompanyScanSettingsInput
	updSettingsErr  error
	deleteID        string
	deleteErr       error
	setAllActiveVal bool
	setAllActiveErr error
}

func (r *recordingAdminForRest) List(_ context.Context, filter companydto.CompanyFilter, page, limit int) (companydto.CompanyAdminListResult, error) {
	r.listFilter = filter
	r.listPage = page
	r.listLimit = limit
	return r.listResult, r.listErr
}
func (r *recordingAdminForRest) Get(_ context.Context, id string) (*companydto.CompanyAdminView, error) {
	r.getID = id
	return r.getView, r.getErr
}
func (r *recordingAdminForRest) Create(_ context.Context, p companydto.CompanyPublicInput, s companydto.CompanyScanSettingsInput) (*companydto.CompanyAdminView, error) {
	r.createPublic = p
	r.createSettings = s
	return r.createView, r.createErr
}
func (r *recordingAdminForRest) UpdatePublic(_ context.Context, id string, p companydto.CompanyPublicInput) error {
	r.updPublicID = id
	r.updPublicInput = p
	return r.updPublicErr
}
func (r *recordingAdminForRest) UpdateScanSettings(_ context.Context, id string, p companydto.CompanyScanSettingsInput) error {
	r.updSettingsID = id
	r.updSettingsIn = p
	return r.updSettingsErr
}
func (r *recordingAdminForRest) Delete(_ context.Context, id string) error {
	r.deleteID = id
	return r.deleteErr
}
func (r *recordingAdminForRest) SetAllActive(_ context.Context, active bool) error {
	r.setAllActiveVal = active
	return r.setAllActiveErr
}
func (r *recordingAdminForRest) GetByIDs(_ context.Context, _ []string) ([]companydto.CompanyAdminView, error) {
	return nil, nil
}
func (r *recordingAdminForRest) GetAll(_ context.Context) ([]companydto.CompanyAdminView, error) {
	return nil, nil
}

func TestAdminRest_List_QueryParsing(t *testing.T) {
	rec := &recordingAdminForRest{listResult: companydto.CompanyAdminListResult{Total: 5, TotalActive: 2}}
	r := newAdminRouter(rec)

	req := httptest.NewRequest(http.MethodGet,
		"/companies?adapter_types=greenhouse&adapter_types=lever&active=true&has_settings=false&search=stripe&sort_by=name&sort_dir=desc&scan_status=ok&page=2&limit=50", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	// Validate every query param landed on the typed filter.
	if got := rec.listFilter.AdapterTypes; len(got) != 2 || got[0] != "greenhouse" || got[1] != "lever" {
		t.Errorf("AdapterTypes = %v, want [greenhouse lever]", got)
	}
	if rec.listFilter.Active == nil || !*rec.listFilter.Active {
		t.Errorf("Active = %v, want true", rec.listFilter.Active)
	}
	if rec.listFilter.HasSettings == nil || *rec.listFilter.HasSettings {
		t.Errorf("HasSettings = %v, want false", rec.listFilter.HasSettings)
	}
	if rec.listFilter.Search != "stripe" || rec.listFilter.SortBy != "name" || rec.listFilter.SortDir != "desc" || rec.listFilter.ScanStatus != "ok" {
		t.Errorf("string filter mismatch: %+v", rec.listFilter)
	}
	if rec.listPage != 2 || rec.listLimit != 50 {
		t.Errorf("page=%d limit=%d, want 2/50", rec.listPage, rec.listLimit)
	}

	// Response envelope shape. handler.Respond wraps every payload in
	// `{data: ...}`, so the admin-list envelope sits one level deeper.
	var body struct {
		Data struct {
			Total       int64                         `json:"total"`
			TotalActive int64                         `json:"total_active"`
			Page        int                           `json:"page"`
			Limit       int                           `json:"limit"`
			Items       []companydto.CompanyAdminView `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Total != 5 || body.Data.TotalActive != 2 || body.Data.Page != 2 || body.Data.Limit != 50 {
		t.Errorf("response envelope wrong: %+v", body)
	}
}

func TestAdminRest_List_SingleAdapterTypeQueryParam(t *testing.T) {
	rec := &recordingAdminForRest{}
	r := newAdminRouter(rec)
	req := httptest.NewRequest(http.MethodGet, "/companies?adapter_type=apple", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := rec.listFilter.AdapterTypes; len(got) != 1 || got[0] != "apple" {
		t.Errorf("AdapterTypes from adapter_type singular = %v, want [apple]", got)
	}
}

func TestAdminRest_List_ServiceError(t *testing.T) {
	rec := &recordingAdminForRest{listErr: apperr.ErrInternal.WithMessage("boom")}
	r := newAdminRouter(rec)
	req := httptest.NewRequest(http.MethodGet, "/companies", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestAdminRest_Get_OK(t *testing.T) {
	rec := &recordingAdminForRest{getView: &companydto.CompanyAdminView{
		Company: companymodel.Company{ID: "stripe", Name: "Stripe", Slug: "stripe"},
	}}
	r := newAdminRouter(rec)
	req := httptest.NewRequest(http.MethodGet, "/companies/stripe", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if rec.getID != "stripe" {
		t.Errorf("getID = %q, want stripe", rec.getID)
	}
}

func TestAdminRest_Get_NotFound(t *testing.T) {
	rec := &recordingAdminForRest{getErr: apperr.ErrNotFound.WithMessage("nope")}
	r := newAdminRouter(rec)
	req := httptest.NewRequest(http.MethodGet, "/companies/stripe", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestAdminRest_Create_InvalidBody(t *testing.T) {
	r := newAdminRouter(&recordingAdminForRest{})
	req := httptest.NewRequest(http.MethodPost, "/companies", strings.NewReader("{not json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAdminRest_Create_ServiceError(t *testing.T) {
	rec := &recordingAdminForRest{createErr: apperr.ErrInternal.WithMessage("boom")}
	r := newAdminRouter(rec)
	body, _ := json.Marshal(map[string]any{"id": "x", "name": "x", "slug": "x"})
	req := httptest.NewRequest(http.MethodPost, "/companies", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestAdminRest_UpdatePublic_InvalidBody(t *testing.T) {
	r := newAdminRouter(&recordingAdminForRest{})
	req := httptest.NewRequest(http.MethodPatch, "/companies/stripe", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAdminRest_UpdatePublic_ServiceError(t *testing.T) {
	rec := &recordingAdminForRest{updPublicErr: apperr.ErrInternal.WithMessage("boom")}
	r := newAdminRouter(rec)
	body, _ := json.Marshal(companydto.CompanyPublicInput{Name: "X"})
	req := httptest.NewRequest(http.MethodPatch, "/companies/stripe", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestAdminRest_UpdateScanSettings_InvalidBody(t *testing.T) {
	r := newAdminRouter(&recordingAdminForRest{})
	req := httptest.NewRequest(http.MethodPatch, "/companies/stripe/scan-settings", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAdminRest_UpdateScanSettings_ServiceError(t *testing.T) {
	rec := &recordingAdminForRest{updSettingsErr: apperr.ErrNotFound.WithMessage("missing")}
	r := newAdminRouter(rec)
	body, _ := json.Marshal(companydto.CompanyScanSettingsInput{Active: boolPtr(false)})
	req := httptest.NewRequest(http.MethodPatch, "/companies/stripe/scan-settings", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestAdminRest_Delete_OK(t *testing.T) {
	rec := &recordingAdminForRest{}
	r := newAdminRouter(rec)
	req := httptest.NewRequest(http.MethodDelete, "/companies/stripe", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if rec.deleteID != "stripe" {
		t.Errorf("deleteID = %q, want stripe", rec.deleteID)
	}
}

func TestAdminRest_SetAllActive_InvalidBody(t *testing.T) {
	r := newAdminRouter(&recordingAdminForRest{})
	req := httptest.NewRequest(http.MethodPatch, "/companies/active", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAdminRest_SetAllActive_ServiceError(t *testing.T) {
	rec := &recordingAdminForRest{setAllActiveErr: apperr.ErrInternal.WithMessage("boom")}
	r := newAdminRouter(rec)
	body, _ := json.Marshal(map[string]bool{"active": true})
	req := httptest.NewRequest(http.MethodPatch, "/companies/active", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// TestAdminRest_Create_SettingsPlumbed verifies the embedded ScanSettings on
// the request body reaches the service layer intact (adapter_type only on
// purpose — partial payloads must round-trip).
func TestAdminRest_Create_SettingsPlumbed(t *testing.T) {
	rec := &recordingAdminForRest{createView: &companydto.CompanyAdminView{Company: companymodel.Company{ID: "x"}}}
	r := newAdminRouter(rec)
	body, _ := json.Marshal(map[string]any{
		"id":   "x",
		"name": "X",
		"slug": "x",
		"scan_settings": map[string]any{
			"adapter_type": companycommon.AdapterMeta,
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/companies", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if rec.createSettings.AdapterType == nil || *rec.createSettings.AdapterType != companycommon.AdapterMeta {
		t.Errorf("AdapterType = %v, want meta", rec.createSettings.AdapterType)
	}
}
