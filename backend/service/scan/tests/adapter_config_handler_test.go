package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// mockAdapterConfigService is a configurable test double for AdapterConfigService.
type mockAdapterConfigService struct {
	listResult   []dto.AdapterConfigDTO
	updateResult dto.AdapterConfigDTO
	configResult dto.EffectiveScanConfigDTO
	err          error
}

func (m *mockAdapterConfigService) ListAdapterConfigs(_ context.Context) ([]dto.AdapterConfigDTO, error) {
	return m.listResult, m.err
}

func (m *mockAdapterConfigService) UpdateAdapterConfig(_ context.Context, _ string, _ dto.UpdateAdapterConfigInput) (dto.AdapterConfigDTO, error) {
	return m.updateResult, m.err
}

func (m *mockAdapterConfigService) GetEffectiveScanConfig(_ context.Context, _ string) (dto.EffectiveScanConfigDTO, error) {
	return m.configResult, m.err
}

func (m *mockAdapterConfigService) UpdateScanIntervalOverride(_ context.Context, _ string, _ *int) error {
	return m.err
}

var _ svc.AdapterConfigService = (*mockAdapterConfigService)(nil)

// newAdapterConfigTestRouter wires the full decorator chain and registers routes.
func newAdapterConfigTestRouter(next svc.AdapterConfigService) *chi.Mux {
	valid := svc.NewAdapterConfigValidator(next)
	h := svc.NewAdapterConfigRestService(valid)
	router := svc.NewAdapterConfigRestRouter(h)
	r := chi.NewRouter()
	router.Register(r)
	return r
}

func TestAdapterConfigHandler_ListAdapterConfigs_OK(t *testing.T) {
	mock := &mockAdapterConfigService{
		listResult: []dto.AdapterConfigDTO{
			{AdapterType: "greenhouse", ScanIntervalMinutes: 1440, InterCompanyDelayS: 3},
		},
	}
	r := newAdapterConfigTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet, "/adapter-configs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"data"`) {
		t.Errorf("response missing data envelope: %s", w.Body.String())
	}
}

func TestAdapterConfigHandler_ListAdapterConfigs_InternalError(t *testing.T) {
	mock := &mockAdapterConfigService{err: apperr.ErrInternal.WithMessage("db down")}
	r := newAdapterConfigTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet, "/adapter-configs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestAdapterConfigHandler_UpdateAdapterConfig_OK(t *testing.T) {
	mock := &mockAdapterConfigService{
		updateResult: dto.AdapterConfigDTO{AdapterType: "greenhouse", ScanIntervalMinutes: 60},
	}
	r := newAdapterConfigTestRouter(mock)

	body, _ := json.Marshal(map[string]int{"scan_interval_minutes": 60})
	req := httptest.NewRequest(http.MethodPatch, "/adapter-configs/greenhouse", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestAdapterConfigHandler_UpdateAdapterConfig_ValidationError(t *testing.T) {
	mock := &mockAdapterConfigService{}
	r := newAdapterConfigTestRouter(mock)

	// Empty body — no fields provided — validation should reject
	body, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodPatch, "/adapter-configs/greenhouse", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestAdapterConfigHandler_UpdateAdapterConfig_NotFound(t *testing.T) {
	mock := &mockAdapterConfigService{err: apperr.ErrNotFound.WithMessage("adapter config not found")}
	r := newAdapterConfigTestRouter(mock)

	body, _ := json.Marshal(map[string]int{"scan_interval_minutes": 60})
	req := httptest.NewRequest(http.MethodPatch, "/adapter-configs/nonexistent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404; body: %s", w.Code, w.Body.String())
	}
}

func TestAdapterConfigHandler_UpdateScanIntervalOverride_OK(t *testing.T) {
	mock := &mockAdapterConfigService{}
	r := newAdapterConfigTestRouter(mock)

	body, _ := json.Marshal(map[string]any{"scan_interval_minutes_override": 60})
	req := httptest.NewRequest(http.MethodPatch, "/companies/company-1/scan-interval-override", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestAdapterConfigHandler_UpdateScanIntervalOverride_ClearOverride(t *testing.T) {
	mock := &mockAdapterConfigService{}
	r := newAdapterConfigTestRouter(mock)

	body, _ := json.Marshal(map[string]any{"scan_interval_minutes_override": nil})
	req := httptest.NewRequest(http.MethodPatch, "/companies/company-1/scan-interval-override", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestAdapterConfigHandler_UpdateScanIntervalOverride_NotFound(t *testing.T) {
	mock := &mockAdapterConfigService{err: apperr.ErrNotFound.WithMessage("company not found")}
	r := newAdapterConfigTestRouter(mock)

	body, _ := json.Marshal(map[string]any{"scan_interval_minutes_override": 60})
	req := httptest.NewRequest(http.MethodPatch, "/companies/missing/scan-interval-override", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404; body: %s", w.Code, w.Body.String())
	}
}

func TestAdapterConfigHandler_GetEffectiveScanConfig_OK(t *testing.T) {
	override := 60
	mock := &mockAdapterConfigService{
		configResult: dto.EffectiveScanConfigDTO{
			CompanyID:                    "company-1",
			AdapterType:                  "greenhouse",
			AdapterScanIntervalMinutes:   1440,
			EffectiveScanIntervalMinutes: 60,
			ScanIntervalMinutesOverride:  &override,
			InterCompanyDelayS:           3,
		},
	}
	r := newAdapterConfigTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet, "/companies/company-1/scan-config", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"effective_scan_interval_minutes"`) {
		t.Errorf("response missing effective_scan_interval_minutes: %s", w.Body.String())
	}
}

func TestAdapterConfigHandler_GetEffectiveScanConfig_NotFound(t *testing.T) {
	mock := &mockAdapterConfigService{err: apperr.ErrNotFound.WithMessage("company not found")}
	r := newAdapterConfigTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet, "/companies/missing/scan-config", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404; body: %s", w.Code, w.Body.String())
	}
}
