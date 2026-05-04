package company_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

func newPublicRouter(svc companysvc.CompanyService) *chi.Mux {
	rest := companysvc.NewRestService(svc)
	router := companysvc.NewRestRouter(rest)
	r := chi.NewRouter()
	router.Register(r)
	return r
}

func TestPublicRest_List_OK(t *testing.T) {
	svc := &fakePublic{listResult: companydto.CompanyPublicListResult{
		Items: []companydto.CompanyPublicView{{Company: companymodel.Company{ID: "stripe", Name: "Stripe", Slug: "stripe"}}},
		Total: 1,
	}}
	r := newPublicRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/companies", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"data"`) {
		t.Errorf("response missing data envelope: %s", w.Body.String())
	}
}

func TestPublicRest_Get_OK(t *testing.T) {
	svc := &fakePublic{getResult: &companydto.CompanyPublicView{Company: companymodel.Company{ID: "stripe", Name: "Stripe", Slug: "stripe"}}}
	r := newPublicRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/companies/stripe", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// TestPublicView_NeverCarriesOperationalFields locks down the public DTO's
// JSON shape. If anyone re-introduces an operational field (adapter_type,
// active, last_scan_at, etc.) on the public view, this test fails — the
// public surface is the boundary that must NEVER leak operational data.
func TestPublicView_NeverCarriesOperationalFields(t *testing.T) {
	v := companydto.CompanyPublicView{Company: companymodel.Company{
		ID:   "stripe",
		Name: "Stripe",
		Slug: "stripe",
	}}
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	forbidden := []string{
		"adapter_type", "adapter_config", "scan_interval_override",
		"active", "last_scan_at", "last_scan_status", "last_success_at",
		"last_error_at", "last_error_message", "consecutive_errors",
		"scan_settings",
	}
	for _, k := range forbidden {
		if _, ok := out[k]; ok {
			t.Errorf("public view leaks forbidden operational field %q (full json: %s)", k, string(body))
		}
	}

	// Sanity: the expected public fields ARE present.
	expected := []string{"id", "name", "slug"}
	for _, k := range expected {
		if _, ok := out[k]; !ok {
			t.Errorf("public view missing expected field %q (full json: %s)", k, string(body))
		}
	}

	// The struct shape itself must not declare ScanSettings — guard against
	// an unintended embedding.
	rt := reflect.TypeOf(companydto.CompanyPublicView{})
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Name == "ScanSettings" || f.Name == "Settings" {
			t.Errorf("public view exposes operational field %q at the struct level", f.Name)
		}
	}
}
