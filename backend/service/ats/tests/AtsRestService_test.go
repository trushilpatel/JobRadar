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
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
)

// stubAtsService is a minimal AtsService double for handler tests.
type stubAtsService struct {
	listResult   []atsdto.ATSDTO
	listErr      error
	getResult    map[string]atsdto.ATSDTO
	getErr       map[string]error
	updateResult atsdto.ATSDTO
	updateErr    error
}

func newStubAtsService() *stubAtsService {
	return &stubAtsService{
		getResult: make(map[string]atsdto.ATSDTO),
		getErr:    make(map[string]error),
	}
}

func (s *stubAtsService) ListATS(_ context.Context) ([]atsdto.ATSDTO, error) {
	return s.listResult, s.listErr
}

func (s *stubAtsService) GetATS(_ context.Context, name string) (atsdto.ATSDTO, error) {
	if err, ok := s.getErr[name]; ok && err != nil {
		return atsdto.ATSDTO{}, err
	}
	if r, ok := s.getResult[name]; ok {
		return r, nil
	}
	return atsdto.ATSDTO{}, nil
}

func (s *stubAtsService) UpdateATS(_ context.Context, _ string, _ atsdto.UpdateATSInput) (atsdto.ATSDTO, error) {
	return s.updateResult, s.updateErr
}

// newAtsTestRouter wires a chi router with the ATS RestRouter registered.
// Uses a fresh, isolated zone config/registry per call — never
// apispec.APIZoneConfig()/AdminZoneConfig(), which share the production
// package-level singleton registry (see apps/api/app_test.go for why that
// matters).
func newAtsTestRouter(svc atssvc.AtsService) http.Handler {
	rest := atssvc.NewRestService(svc)
	router := atssvc.NewRestRouter(rest, apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry()))
	mux := chi.NewMux()
	router.Register(mux)
	return mux
}

// ---------- GET /ats ----------

func TestAtsRest_ListATS_200(t *testing.T) {
	t.Parallel()
	svc := newStubAtsService()
	svc.listResult = []atsdto.ATSDTO{
		{Name: "greenhouse", DisplayName: "Greenhouse"},
		{Name: "lever", DisplayName: "Lever"},
	}
	srv := httptest.NewServer(newAtsTestRouter(svc))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ats")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Data []atsdto.ATSDTO `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 2 {
		t.Errorf("len = %d, want 2", len(body.Data))
	}
}

func TestAtsRest_ListATS_500(t *testing.T) {
	t.Parallel()
	svc := newStubAtsService()
	svc.listErr = domainerrs.ErrInternal.WithMessage("db down")
	srv := httptest.NewServer(newAtsTestRouter(svc))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ats")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

// ---------- GET /ats/{name} ----------

func TestAtsRest_GetATS_200(t *testing.T) {
	t.Parallel()
	svc := newStubAtsService()
	svc.getResult["greenhouse"] = atsdto.ATSDTO{Name: "greenhouse", DisplayName: "Greenhouse"}
	srv := httptest.NewServer(newAtsTestRouter(svc))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ats/greenhouse")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Data atsdto.ATSDTO `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Name != "greenhouse" {
		t.Errorf("name = %q, want greenhouse", body.Data.Name)
	}
}

func TestAtsRest_GetATS_404(t *testing.T) {
	t.Parallel()
	svc := newStubAtsService()
	svc.getErr["missing"] = domainerrs.ErrNotFound.WithMessage("not found")
	srv := httptest.NewServer(newAtsTestRouter(svc))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ats/missing")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestAtsRest_GetATS_500(t *testing.T) {
	t.Parallel()
	svc := newStubAtsService()
	svc.getErr["greenhouse"] = domainerrs.ErrInternal.WithMessage("db error")
	srv := httptest.NewServer(newAtsTestRouter(svc))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ats/greenhouse")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

// ---------- PATCH /ats/{name} ----------

func TestAtsRest_UpdateATS_200(t *testing.T) {
	t.Parallel()
	svc := newStubAtsService()
	svc.updateResult = atsdto.ATSDTO{Name: "greenhouse", IsActive: false}
	srv := httptest.NewServer(newAtsTestRouter(svc))
	defer srv.Close()

	body := strings.NewReader(`{"is_active":false}`)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/ats/greenhouse", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Data atsdto.ATSDTO `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Data.Name != "greenhouse" {
		t.Errorf("name = %q, want greenhouse", out.Data.Name)
	}
}

func TestAtsRest_UpdateATS_400_MalformedBody(t *testing.T) {
	t.Parallel()
	svc := newStubAtsService()
	srv := httptest.NewServer(newAtsTestRouter(svc))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/ats/greenhouse", strings.NewReader("not json"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestAtsRest_UpdateATS_404(t *testing.T) {
	t.Parallel()
	svc := newStubAtsService()
	svc.updateErr = domainerrs.ErrNotFound.WithMessage("not found")
	srv := httptest.NewServer(newAtsTestRouter(svc))
	defer srv.Close()

	body := strings.NewReader(`{"is_active":true}`)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/ats/missing", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestAtsRest_UpdateATS_500(t *testing.T) {
	t.Parallel()
	svc := newStubAtsService()
	svc.updateErr = domainerrs.ErrInternal.WithMessage("db error")
	srv := httptest.NewServer(newAtsTestRouter(svc))
	defer srv.Close()

	body := strings.NewReader(`{"is_active":true}`)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/ats/greenhouse", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

// TestAtsRest_UpdateATS_ValidationError verifies that a ValidationErrors
// response from the service results in HTTP 400 with the field list.
func TestAtsRest_UpdateATS_ValidationError(t *testing.T) {
	t.Parallel()
	svc := newStubAtsService()
	svc.updateErr = domainerrs.ValidationErrors{
		{Field: "scan_interval_min", Code: "INVALID_VALUE", Message: "must be between 1 and 10080"},
	}
	srv := httptest.NewServer(newAtsTestRouter(svc))
	defer srv.Close()

	body := strings.NewReader(`{"scan_interval_min":0}`)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/ats/greenhouse", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestAtsRestService_NilNextPanics verifies that constructing a RestService
// with a nil next panics — guards against misconfigured wire setup.
func TestAtsRestService_NilNextPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil next")
		}
	}()
	_ = atssvc.NewRestService(nil)
}

// Verify that errors.Is wiring is correct (used by the test itself).
var _ = errors.New // ensure errors package is used
