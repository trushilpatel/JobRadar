package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/go-chi/chi/v5"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// fakeAtsService is a test double for atssvc.AtsService.
// Known adapters return a valid DTO; unknown names return ErrNotFound.
type fakeAtsService struct {
	known map[string]atsdto.ATSDTO
	err   error // if non-nil, all calls return this error
}

func newFakeAts(adapters ...atsdto.ATSDTO) *fakeAtsService {
	m := make(map[string]atsdto.ATSDTO, len(adapters))
	for _, a := range adapters {
		m[a.Name] = a
	}
	return &fakeAtsService{known: m}
}

func (f *fakeAtsService) ListATS(_ context.Context) ([]atsdto.ATSDTO, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]atsdto.ATSDTO, 0, len(f.known))
	for _, v := range f.known {
		out = append(out, v)
	}
	return out, nil
}

func (f *fakeAtsService) GetATS(_ context.Context, name string) (atsdto.ATSDTO, error) {
	if f.err != nil {
		return atsdto.ATSDTO{}, f.err
	}
	d, ok := f.known[name]
	if !ok {
		return atsdto.ATSDTO{}, apperr.ErrNotFound.WithMessage("ats not found: " + name)
	}
	return d, nil
}

func (f *fakeAtsService) UpdateATS(_ context.Context, _ string, _ atsdto.UpdateATSInput) (atsdto.ATSDTO, error) {
	return atsdto.ATSDTO{}, nil
}

var _ atssvc.AtsService = (*fakeAtsService)(nil)

func ptr(i int) *int { return &i }

// newScanConfigTestDB returns an in-memory SQLite DB with the
// company_scan_settings table migrated. Used for repository-level tests of
// GetEffectiveScanConfig and UpdateScanIntervalOverride.
func newScanConfigTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&companymodel.CompanyScanSettings{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// closedScanConfigDB returns a DB whose connection is closed so every
// subsequent operation surfaces an error — used to exercise error branches.
func closedScanConfigDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newScanConfigTestDB(t)
	raw, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

// newRepoWithDB builds a repository service with a real SQLite DB and the
// given AtsService — used to exercise GetEffectiveScanConfig and
// UpdateScanIntervalOverride end-to-end.
func newRepoWithDB(db *gorm.DB, atsFake atssvc.AtsService) svc.Service {
	return svc.NewRepositoryService(&fakeQueueStore{}, &fakeScanLogReader{}, svc.Config{}, db, atsFake)
}

// --- GetEffectiveScanConfig ---

func TestGetEffectiveScanConfig_WithOverride(t *testing.T) {
	db := newScanConfigTestDB(t)
	override := 30
	if err := db.Create(&companymodel.CompanyScanSettings{
		CompanyID:            "stripe",
		AdapterType:          "greenhouse",
		ScanIntervalOverride: &override,
		Active:               true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	ats := newFakeAts(atsdto.ATSDTO{Name: "greenhouse", ScanIntervalMin: 1440, InterCompanyDelayS: 3})
	repo := newRepoWithDB(db, ats)

	cfg, err := repo.GetEffectiveScanConfig(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("GetEffectiveScanConfig() error = %v", err)
	}
	if cfg.EffectiveScanIntervalMinutes != 30 {
		t.Errorf("effective = %d, want 30 (override wins)", cfg.EffectiveScanIntervalMinutes)
	}
	if cfg.AdapterScanIntervalMinutes != 1440 {
		t.Errorf("adapter default = %d, want 1440", cfg.AdapterScanIntervalMinutes)
	}
	if cfg.AdapterType != "greenhouse" {
		t.Errorf("adapter_type = %q, want greenhouse", cfg.AdapterType)
	}
	if cfg.InterCompanyDelayS != 3 {
		t.Errorf("inter_company_delay_s = %d, want 3", cfg.InterCompanyDelayS)
	}
}

func TestGetEffectiveScanConfig_NoOverride(t *testing.T) {
	db := newScanConfigTestDB(t)
	if err := db.Create(&companymodel.CompanyScanSettings{
		CompanyID:   "stripe",
		AdapterType: "greenhouse",
		Active:      true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	ats := newFakeAts(atsdto.ATSDTO{Name: "greenhouse", ScanIntervalMin: 1440, InterCompanyDelayS: 5})
	repo := newRepoWithDB(db, ats)

	cfg, err := repo.GetEffectiveScanConfig(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("GetEffectiveScanConfig() error = %v", err)
	}
	if cfg.EffectiveScanIntervalMinutes != 1440 {
		t.Errorf("effective = %d, want 1440 (no override)", cfg.EffectiveScanIntervalMinutes)
	}
	if cfg.ScanIntervalMinutesOverride != nil {
		t.Errorf("expected nil override, got %v", *cfg.ScanIntervalMinutesOverride)
	}
}

func TestGetEffectiveScanConfig_SettingsNotFound(t *testing.T) {
	db := newScanConfigTestDB(t)
	ats := newFakeAts(atsdto.ATSDTO{Name: "greenhouse", ScanIntervalMin: 1440})
	repo := newRepoWithDB(db, ats)

	_, err := repo.GetEffectiveScanConfig(context.Background(), "missing")
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %v", err)
	}
}

func TestGetEffectiveScanConfig_AtsNotFound(t *testing.T) {
	db := newScanConfigTestDB(t)
	if err := db.Create(&companymodel.CompanyScanSettings{
		CompanyID:   "stripe",
		AdapterType: "exotic",
		Active:      true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	// empty AtsService — exotic is not registered
	repo := newRepoWithDB(db, newFakeAts())

	_, err := repo.GetEffectiveScanConfig(context.Background(), "stripe")
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND (ats missing), got %v", err)
	}
}

func TestGetEffectiveScanConfig_DBError(t *testing.T) {
	repo := newRepoWithDB(closedScanConfigDB(t), newFakeAts())

	_, err := repo.GetEffectiveScanConfig(context.Background(), "stripe")
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL on closed DB, got %v", err)
	}
}

func TestGetEffectiveScanConfig_AtsServiceError(t *testing.T) {
	db := newScanConfigTestDB(t)
	if err := db.Create(&companymodel.CompanyScanSettings{
		CompanyID:   "stripe",
		AdapterType: "greenhouse",
		Active:      true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	errFake := &fakeAtsService{err: apperr.ErrInternal.WithMessage("ats down")}
	repo := newRepoWithDB(db, errFake)

	_, err := repo.GetEffectiveScanConfig(context.Background(), "stripe")
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL from ats error, got %v", err)
	}
}

// --- UpdateScanIntervalOverride ---

func TestUpdateScanIntervalOverride_OK(t *testing.T) {
	db := newScanConfigTestDB(t)
	if err := db.Create(&companymodel.CompanyScanSettings{
		CompanyID:   "stripe",
		AdapterType: "greenhouse",
		Active:      true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := newRepoWithDB(db, newFakeAts())

	v := 60
	if err := repo.UpdateScanIntervalOverride(context.Background(), "stripe", &v); err != nil {
		t.Fatalf("UpdateScanIntervalOverride() error = %v", err)
	}

	var got companymodel.CompanyScanSettings
	if err := db.First(&got, "company_id = ?", "stripe").Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.ScanIntervalOverride == nil || *got.ScanIntervalOverride != 60 {
		t.Errorf("expected override=60, got %v", got.ScanIntervalOverride)
	}
}

func TestUpdateScanIntervalOverride_Clear(t *testing.T) {
	db := newScanConfigTestDB(t)
	prev := 30
	if err := db.Create(&companymodel.CompanyScanSettings{
		CompanyID:            "stripe",
		AdapterType:          "greenhouse",
		ScanIntervalOverride: &prev,
		Active:               true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := newRepoWithDB(db, newFakeAts())

	if err := repo.UpdateScanIntervalOverride(context.Background(), "stripe", nil); err != nil {
		t.Fatalf("UpdateScanIntervalOverride(nil) error = %v", err)
	}

	var got companymodel.CompanyScanSettings
	if err := db.First(&got, "company_id = ?", "stripe").Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.ScanIntervalOverride != nil {
		t.Errorf("expected nil override after clear, got %v", *got.ScanIntervalOverride)
	}
}

func TestUpdateScanIntervalOverride_NotFound(t *testing.T) {
	db := newScanConfigTestDB(t)
	repo := newRepoWithDB(db, newFakeAts())

	v := 30
	err := repo.UpdateScanIntervalOverride(context.Background(), "missing", &v)
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %v", err)
	}
}

func TestUpdateScanIntervalOverride_DBError(t *testing.T) {
	repo := newRepoWithDB(closedScanConfigDB(t), newFakeAts())

	v := 30
	err := repo.UpdateScanIntervalOverride(context.Background(), "stripe", &v)
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL on closed DB, got %v", err)
	}
}

// --- Validation ---

func TestScanValidator_GetEffectiveScanConfig_EmptyCompanyID(t *testing.T) {
	v := svc.NewValidator(newRepoWithDB(newScanConfigTestDB(t), newFakeAts()))
	_, err := v.GetEffectiveScanConfig(context.Background(), "")
	var vErrs apperr.ValidationErrors
	if !errors.As(err, &vErrs) {
		t.Errorf("expected ValidationErrors, got %T: %v", err, err)
	}
}

func TestScanValidator_GetEffectiveScanConfig_ValidID_Delegates(t *testing.T) {
	db := newScanConfigTestDB(t)
	if err := db.Create(&companymodel.CompanyScanSettings{
		CompanyID:   "stripe",
		AdapterType: "greenhouse",
		Active:      true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	ats := newFakeAts(atsdto.ATSDTO{Name: "greenhouse", ScanIntervalMin: 1440})
	v := svc.NewValidator(newRepoWithDB(db, ats))

	cfg, err := v.GetEffectiveScanConfig(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("GetEffectiveScanConfig() error = %v", err)
	}
	if cfg.CompanyID != "stripe" {
		t.Errorf("company_id = %q, want stripe", cfg.CompanyID)
	}
}

func TestScanValidator_UpdateScanIntervalOverride_EmptyCompanyID(t *testing.T) {
	v := svc.NewValidator(newRepoWithDB(newScanConfigTestDB(t), newFakeAts()))
	err := v.UpdateScanIntervalOverride(context.Background(), "", ptr(60))
	var vErrs apperr.ValidationErrors
	if !errors.As(err, &vErrs) {
		t.Errorf("expected ValidationErrors for empty company_id, got %T: %v", err, err)
	}
}

func TestScanValidator_UpdateScanIntervalOverride_ZeroOverrideInvalid(t *testing.T) {
	v := svc.NewValidator(newRepoWithDB(newScanConfigTestDB(t), newFakeAts()))
	err := v.UpdateScanIntervalOverride(context.Background(), "stripe", ptr(0))
	var vErrs apperr.ValidationErrors
	if !errors.As(err, &vErrs) {
		t.Errorf("expected ValidationErrors for override=0, got %T: %v", err, err)
	}
}

func TestScanValidator_UpdateScanIntervalOverride_AboveMaxInvalid(t *testing.T) {
	v := svc.NewValidator(newRepoWithDB(newScanConfigTestDB(t), newFakeAts()))
	err := v.UpdateScanIntervalOverride(context.Background(), "stripe", ptr(10081))
	var vErrs apperr.ValidationErrors
	if !errors.As(err, &vErrs) {
		t.Errorf("expected ValidationErrors for override=10081, got %T: %v", err, err)
	}
}

func TestScanValidator_UpdateScanIntervalOverride_NilClears(t *testing.T) {
	db := newScanConfigTestDB(t)
	prev := 30
	if err := db.Create(&companymodel.CompanyScanSettings{
		CompanyID:            "stripe",
		AdapterType:          "greenhouse",
		ScanIntervalOverride: &prev,
		Active:               true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	v := svc.NewValidator(newRepoWithDB(db, newFakeAts()))
	if err := v.UpdateScanIntervalOverride(context.Background(), "stripe", nil); err != nil {
		t.Errorf("expected nil error for nil override (clear): %v", err)
	}
}

func TestScanValidator_UpdateScanIntervalOverride_ValidOverride(t *testing.T) {
	db := newScanConfigTestDB(t)
	if err := db.Create(&companymodel.CompanyScanSettings{
		CompanyID:   "stripe",
		AdapterType: "greenhouse",
		Active:      true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	v := svc.NewValidator(newRepoWithDB(db, newFakeAts()))
	if err := v.UpdateScanIntervalOverride(context.Background(), "stripe", ptr(60)); err != nil {
		t.Errorf("expected no error for valid override=60: %v", err)
	}
}

// --- HTTP handler ---

// newScanConfigTestRouter wires the full scan decorator chain and returns a
// router that exercises the two new HTTP endpoints.
func newScanConfigTestRouter(next svc.Service) *chi.Mux {
	valid := svc.NewValidator(next)
	h := svc.NewRestService(valid)
	router := svc.NewRestRouter(h)
	r := chi.NewRouter()
	router.Register(r)
	return r
}

// mockScanService is a minimal Service test double for handler tests.
type mockScanService struct {
	configResult dto.EffectiveScanConfigDTO
	err          error
}

func (m *mockScanService) EnqueueDue(_ context.Context, _ int) (svc.SchedulerCycleResult, error) {
	return svc.SchedulerCycleResult{}, m.err
}
func (m *mockScanService) ResetStuck(_ context.Context, _ time.Duration) (int, error) {
	return 0, m.err
}
func (m *mockScanService) PurgeInactive(_ context.Context) (int, error)  { return 0, m.err }
func (m *mockScanService) TriggerScan(_ context.Context, _ string) error { return m.err }
func (m *mockScanService) TriggerScanAll(_ context.Context) (int, error) { return 0, m.err }
func (m *mockScanService) GetStats(_ context.Context, _ string) (*dto.ScanStatsView, error) {
	return nil, m.err
}
func (m *mockScanService) GetAnalytics(_ context.Context, _ string) (*dto.CompanyAnalytics, error) {
	return nil, m.err
}
func (m *mockScanService) ListQueue(_ context.Context, _ dto.QueueFilter) ([]dto.QueueEntryView, error) {
	return nil, m.err
}
func (m *mockScanService) RemoveFromQueue(_ context.Context, _ string) error { return m.err }
func (m *mockScanService) GetEffectiveScanConfig(_ context.Context, _ string) (dto.EffectiveScanConfigDTO, error) {
	return m.configResult, m.err
}
func (m *mockScanService) UpdateScanIntervalOverride(_ context.Context, _ string, _ *int) error {
	return m.err
}

var _ svc.Service = (*mockScanService)(nil)

func TestScanHandler_GetEffectiveScanConfig_OK(t *testing.T) {
	override := 60
	mock := &mockScanService{
		configResult: dto.EffectiveScanConfigDTO{
			CompanyID:                    "stripe",
			AdapterType:                  "greenhouse",
			AdapterScanIntervalMinutes:   1440,
			EffectiveScanIntervalMinutes: 60,
			ScanIntervalMinutesOverride:  &override,
			InterCompanyDelayS:           3,
		},
	}
	r := newScanConfigTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet, "/companies/stripe/scan-config", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"effective_scan_interval_minutes"`) {
		t.Errorf("response missing effective_scan_interval_minutes: %s", w.Body.String())
	}
}

func TestScanHandler_GetEffectiveScanConfig_NotFound(t *testing.T) {
	mock := &mockScanService{err: apperr.ErrNotFound.WithMessage("company not found")}
	r := newScanConfigTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet, "/companies/missing/scan-config", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", w.Code, w.Body.String())
	}
}

func TestScanHandler_UpdateScanIntervalOverride_OK(t *testing.T) {
	mock := &mockScanService{}
	r := newScanConfigTestRouter(mock)

	body, _ := json.Marshal(map[string]any{"scan_interval_minutes_override": 60})
	req := httptest.NewRequest(http.MethodPatch, "/companies/stripe/scan-interval-override", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestScanHandler_UpdateScanIntervalOverride_ClearOverride(t *testing.T) {
	mock := &mockScanService{}
	r := newScanConfigTestRouter(mock)

	body, _ := json.Marshal(map[string]any{"scan_interval_minutes_override": nil})
	req := httptest.NewRequest(http.MethodPatch, "/companies/stripe/scan-interval-override", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestScanHandler_UpdateScanIntervalOverride_BadJSON(t *testing.T) {
	mock := &mockScanService{}
	r := newScanConfigTestRouter(mock)

	req := httptest.NewRequest(http.MethodPatch, "/companies/stripe/scan-interval-override", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestScanHandler_UpdateScanIntervalOverride_NotFound(t *testing.T) {
	mock := &mockScanService{err: apperr.ErrNotFound.WithMessage("company not found")}
	r := newScanConfigTestRouter(mock)

	body, _ := json.Marshal(map[string]any{"scan_interval_minutes_override": 60})
	req := httptest.NewRequest(http.MethodPatch, "/companies/missing/scan-interval-override", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", w.Code, w.Body.String())
	}
}
