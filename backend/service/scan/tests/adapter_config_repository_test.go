package scan

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	scanmodel "github.com/trushilpatel/jobradar/service/scan/models"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// newAdapterConfigTestDB returns an in-memory sqlite DB with the
// `adapter_configs` and `company_scan_settings` tables migrated from the
// GORM models. The two methods in adapterConfigStore that read these tables
// rely only on the GORM helpers (First/Find/Updates/Update) — none use raw
// Postgres SQL — so sqlite is a faithful enough backend for unit tests.
func newAdapterConfigTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&scanmodel.AdapterConfig{}, &companymodel.CompanyScanSettings{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// closedDBForScan returns a DB whose underlying connection has been closed
// — every subsequent operation surfaces "sql: database is closed" so the
// repository's error branches are exercised without flakiness.
func closedDBForScan(t *testing.T) *gorm.DB {
	t.Helper()
	db := newAdapterConfigTestDB(t)
	raw, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

// ---------- AdapterConfigStore ----------

func TestAdapterConfigStore_ListConfigs_OrderedByType(t *testing.T) {
	db := newAdapterConfigTestDB(t)
	store := svc.NewAdapterConfigStore(db)

	// Insert out-of-order so we can verify ascending sort.
	for _, t := range []string{"meta", "ashby", "greenhouse"} {
		if err := db.Create(&scanmodel.AdapterConfig{AdapterType: t, ScanIntervalMinutes: 60, InterCompanyDelayS: 3}).Error; err != nil {
			panic(err)
		}
	}

	configs, err := store.ListConfigs(context.Background())
	if err != nil {
		t.Fatalf("ListConfigs() error = %v", err)
	}
	if len(configs) != 3 {
		t.Fatalf("expected 3 configs, got %d", len(configs))
	}
	want := []string{"ashby", "greenhouse", "meta"}
	for i, c := range configs {
		if c.AdapterType != want[i] {
			t.Errorf("position %d: got %q, want %q", i, c.AdapterType, want[i])
		}
	}
}

func TestAdapterConfigStore_ListConfigs_DBError(t *testing.T) {
	store := svc.NewAdapterConfigStore(closedDBForScan(t))

	if _, err := store.ListConfigs(context.Background()); err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestAdapterConfigStore_GetConfig_Found(t *testing.T) {
	db := newAdapterConfigTestDB(t)
	store := svc.NewAdapterConfigStore(db)
	if err := db.Create(&scanmodel.AdapterConfig{AdapterType: "greenhouse", ScanIntervalMinutes: 1440, InterCompanyDelayS: 3}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	c, err := store.GetConfig(context.Background(), "greenhouse")
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
	if c.AdapterType != "greenhouse" || c.ScanIntervalMinutes != 1440 {
		t.Errorf("unexpected config: %+v", c)
	}
}

// TestAdapterConfigStore_GetConfig_NotFound asserts the GORM-native sentinel
// surfaces — the wrapper service maps it to ErrNotFound, so this method
// itself must NOT pre-wrap.
func TestAdapterConfigStore_GetConfig_NotFound(t *testing.T) {
	store := svc.NewAdapterConfigStore(newAdapterConfigTestDB(t))

	_, err := store.GetConfig(context.Background(), "unknown")
	if !apperr.IsNotFound(err) {
		t.Errorf("expected gorm.ErrRecordNotFound, got %v", err)
	}
}

func TestAdapterConfigStore_UpdateConfig_OK(t *testing.T) {
	db := newAdapterConfigTestDB(t)
	store := svc.NewAdapterConfigStore(db)
	if err := db.Create(&scanmodel.AdapterConfig{AdapterType: "greenhouse", ScanIntervalMinutes: 60, InterCompanyDelayS: 3}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	updated, err := store.UpdateConfig(context.Background(), "greenhouse", map[string]any{
		"scan_interval_minutes": 120,
		"inter_company_delay_s": 10,
	})
	if err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}
	if updated.ScanIntervalMinutes != 120 || updated.InterCompanyDelayS != 10 {
		t.Errorf("unexpected updated row: %+v", updated)
	}
}

// TestAdapterConfigStore_UpdateConfig_NotFound asserts the store returns
// gorm.ErrRecordNotFound when no row matches — important so the wrapper
// service can map it to a 404 instead of a 500.
func TestAdapterConfigStore_UpdateConfig_NotFound(t *testing.T) {
	store := svc.NewAdapterConfigStore(newAdapterConfigTestDB(t))

	_, err := store.UpdateConfig(context.Background(), "missing", map[string]any{"scan_interval_minutes": 60})
	if !apperr.IsNotFound(err) {
		t.Errorf("expected ErrRecordNotFound, got %v", err)
	}
}

func TestAdapterConfigStore_UpdateConfig_DBError(t *testing.T) {
	store := svc.NewAdapterConfigStore(closedDBForScan(t))

	_, err := store.UpdateConfig(context.Background(), "greenhouse", map[string]any{"scan_interval_minutes": 60})
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
	if apperr.IsNotFound(err) {
		t.Errorf("unexpected not-found classification: %v", err)
	}
}

func TestAdapterConfigStore_GetCompanyScanSettings_Found(t *testing.T) {
	db := newAdapterConfigTestDB(t)
	store := svc.NewAdapterConfigStore(db)
	override := 30
	if err := db.Create(&companymodel.CompanyScanSettings{CompanyID: "stripe", AdapterType: "greenhouse", ScanIntervalOverride: &override, Active: true}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	s, err := store.GetCompanyScanSettings(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("GetCompanyScanSettings() error = %v", err)
	}
	if s.AdapterType != "greenhouse" || s.ScanIntervalOverride == nil || *s.ScanIntervalOverride != 30 {
		t.Errorf("unexpected settings: %+v", s)
	}
}

func TestAdapterConfigStore_GetCompanyScanSettings_NotFound(t *testing.T) {
	store := svc.NewAdapterConfigStore(newAdapterConfigTestDB(t))

	_, err := store.GetCompanyScanSettings(context.Background(), "missing")
	if !apperr.IsNotFound(err) {
		t.Errorf("expected ErrRecordNotFound, got %v", err)
	}
}

func TestAdapterConfigStore_UpdateCompanyScanIntervalOverride_OK(t *testing.T) {
	db := newAdapterConfigTestDB(t)
	store := svc.NewAdapterConfigStore(db)
	if err := db.Create(&companymodel.CompanyScanSettings{CompanyID: "stripe", AdapterType: "greenhouse", Active: true}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	override := 45
	if err := store.UpdateCompanyScanIntervalOverride(context.Background(), "stripe", &override); err != nil {
		t.Fatalf("UpdateCompanyScanIntervalOverride() error = %v", err)
	}

	// Reload and verify
	var got companymodel.CompanyScanSettings
	if err := db.First(&got, "company_id = ?", "stripe").Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.ScanIntervalOverride == nil || *got.ScanIntervalOverride != 45 {
		t.Errorf("expected override=45, got %v", got.ScanIntervalOverride)
	}
}

// TestAdapterConfigStore_UpdateCompanyScanIntervalOverride_Clear asserts that
// passing nil writes NULL — the API contract for "clear the override".
func TestAdapterConfigStore_UpdateCompanyScanIntervalOverride_Clear(t *testing.T) {
	db := newAdapterConfigTestDB(t)
	store := svc.NewAdapterConfigStore(db)
	override := 30
	if err := db.Create(&companymodel.CompanyScanSettings{CompanyID: "stripe", AdapterType: "greenhouse", ScanIntervalOverride: &override, Active: true}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := store.UpdateCompanyScanIntervalOverride(context.Background(), "stripe", nil); err != nil {
		t.Fatalf("clear override: %v", err)
	}

	var got companymodel.CompanyScanSettings
	if err := db.First(&got, "company_id = ?", "stripe").Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.ScanIntervalOverride != nil {
		t.Errorf("expected nil override after clear, got %v", *got.ScanIntervalOverride)
	}
}

func TestAdapterConfigStore_UpdateCompanyScanIntervalOverride_NotFound(t *testing.T) {
	store := svc.NewAdapterConfigStore(newAdapterConfigTestDB(t))

	override := 30
	err := store.UpdateCompanyScanIntervalOverride(context.Background(), "missing", &override)
	if !apperr.IsNotFound(err) {
		t.Errorf("expected ErrRecordNotFound, got %v", err)
	}
}

func TestAdapterConfigStore_UpdateCompanyScanIntervalOverride_DBError(t *testing.T) {
	store := svc.NewAdapterConfigStore(closedDBForScan(t))

	override := 30
	err := store.UpdateCompanyScanIntervalOverride(context.Background(), "stripe", &override)
	if err == nil {
		t.Fatal("expected error")
	}
	if apperr.IsNotFound(err) {
		t.Errorf("unexpected not-found classification: %v", err)
	}
}

// ---------- adapterConfigRepositoryService ----------
//
// The repository service wraps the store and applies domain error mapping.
// These tests exercise the full service+store path against sqlite so the
// wrapper, the SQL, and the DTO conversion are all covered together.

func TestAdapterConfigRepo_ListAdapterConfigs_DTOConversion(t *testing.T) {
	db := newAdapterConfigTestDB(t)
	store := svc.NewAdapterConfigStore(db)
	repo := svc.NewAdapterConfigRepositoryService(store)
	last := time.Now().UTC().Truncate(time.Second)
	if err := db.Create(&scanmodel.AdapterConfig{AdapterType: "greenhouse", ScanIntervalMinutes: 60, InterCompanyDelayS: 3, LastScheduledAt: &last}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	configs, err := repo.ListAdapterConfigs(context.Background())
	if err != nil {
		t.Fatalf("ListAdapterConfigs() error = %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	if configs[0].AdapterType != "greenhouse" {
		t.Errorf("unexpected adapter type: %q", configs[0].AdapterType)
	}
	if configs[0].LastScheduledAt == nil {
		t.Error("expected LastScheduledAt to be propagated through DTO")
	}
}

func TestAdapterConfigRepo_ListAdapterConfigs_DBError(t *testing.T) {
	store := svc.NewAdapterConfigStore(closedDBForScan(t))
	repo := svc.NewAdapterConfigRepositoryService(store)

	_, err := repo.ListAdapterConfigs(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %v", err)
	}
}

func TestAdapterConfigRepo_UpdateAdapterConfig_OK(t *testing.T) {
	db := newAdapterConfigTestDB(t)
	store := svc.NewAdapterConfigStore(db)
	repo := svc.NewAdapterConfigRepositoryService(store)
	if err := db.Create(&scanmodel.AdapterConfig{AdapterType: "greenhouse", ScanIntervalMinutes: 60, InterCompanyDelayS: 3}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	v := 120
	delay := 7
	updated, err := repo.UpdateAdapterConfig(context.Background(), "greenhouse", dto.UpdateAdapterConfigInput{
		ScanIntervalMinutes: &v,
		InterCompanyDelayS:  &delay,
	})
	if err != nil {
		t.Fatalf("UpdateAdapterConfig() error = %v", err)
	}
	if updated.ScanIntervalMinutes != 120 || updated.InterCompanyDelayS != 7 {
		t.Errorf("unexpected: %+v", updated)
	}
}

func TestAdapterConfigRepo_UpdateAdapterConfig_NotFoundMapping(t *testing.T) {
	store := svc.NewAdapterConfigStore(newAdapterConfigTestDB(t))
	repo := svc.NewAdapterConfigRepositoryService(store)

	v := 60
	_, err := repo.UpdateAdapterConfig(context.Background(), "missing", dto.UpdateAdapterConfigInput{
		ScanIntervalMinutes: &v,
	})
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %v", err)
	}
}

func TestAdapterConfigRepo_UpdateAdapterConfig_DBError(t *testing.T) {
	store := svc.NewAdapterConfigStore(closedDBForScan(t))
	repo := svc.NewAdapterConfigRepositoryService(store)

	v := 60
	_, err := repo.UpdateAdapterConfig(context.Background(), "greenhouse", dto.UpdateAdapterConfigInput{
		ScanIntervalMinutes: &v,
	})
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %v", err)
	}
}

func TestAdapterConfigRepo_GetEffectiveScanConfig_WithOverride(t *testing.T) {
	db := newAdapterConfigTestDB(t)
	store := svc.NewAdapterConfigStore(db)
	repo := svc.NewAdapterConfigRepositoryService(store)
	override := 30
	if err := db.Create(&companymodel.CompanyScanSettings{CompanyID: "stripe", AdapterType: "greenhouse", ScanIntervalOverride: &override, Active: true}).Error; err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	if err := db.Create(&scanmodel.AdapterConfig{AdapterType: "greenhouse", ScanIntervalMinutes: 1440, InterCompanyDelayS: 3}).Error; err != nil {
		t.Fatalf("seed adapter: %v", err)
	}

	cfg, err := repo.GetEffectiveScanConfig(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("GetEffectiveScanConfig() error = %v", err)
	}
	if cfg.EffectiveScanIntervalMinutes != 30 {
		t.Errorf("expected effective=30 (override wins), got %d", cfg.EffectiveScanIntervalMinutes)
	}
	if cfg.AdapterScanIntervalMinutes != 1440 {
		t.Errorf("expected adapter default=1440, got %d", cfg.AdapterScanIntervalMinutes)
	}
}

func TestAdapterConfigRepo_GetEffectiveScanConfig_NoOverride(t *testing.T) {
	db := newAdapterConfigTestDB(t)
	store := svc.NewAdapterConfigStore(db)
	repo := svc.NewAdapterConfigRepositoryService(store)
	if err := db.Create(&companymodel.CompanyScanSettings{CompanyID: "stripe", AdapterType: "greenhouse", Active: true}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Create(&scanmodel.AdapterConfig{AdapterType: "greenhouse", ScanIntervalMinutes: 1440, InterCompanyDelayS: 3}).Error; err != nil {
		t.Fatalf("seed adapter: %v", err)
	}

	cfg, err := repo.GetEffectiveScanConfig(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("GetEffectiveScanConfig() error = %v", err)
	}
	if cfg.EffectiveScanIntervalMinutes != 1440 {
		t.Errorf("expected effective=1440 (no override), got %d", cfg.EffectiveScanIntervalMinutes)
	}
	if cfg.ScanIntervalMinutesOverride != nil {
		t.Errorf("expected nil override, got %v", cfg.ScanIntervalMinutesOverride)
	}
}

func TestAdapterConfigRepo_GetEffectiveScanConfig_SettingsMissing(t *testing.T) {
	store := svc.NewAdapterConfigStore(newAdapterConfigTestDB(t))
	repo := svc.NewAdapterConfigRepositoryService(store)

	_, err := repo.GetEffectiveScanConfig(context.Background(), "missing")
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %v", err)
	}
}

// TestAdapterConfigRepo_GetEffectiveScanConfig_AdapterMissing covers the
// case where the company has a scan_settings row pointing at an
// adapter_type with no global config — surfaced as NOT_FOUND.
func TestAdapterConfigRepo_GetEffectiveScanConfig_AdapterMissing(t *testing.T) {
	db := newAdapterConfigTestDB(t)
	store := svc.NewAdapterConfigStore(db)
	repo := svc.NewAdapterConfigRepositoryService(store)
	if err := db.Create(&companymodel.CompanyScanSettings{CompanyID: "stripe", AdapterType: "exotic", Active: true}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, err := repo.GetEffectiveScanConfig(context.Background(), "stripe")
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %v", err)
	}
}

func TestAdapterConfigRepo_GetEffectiveScanConfig_SettingsDBError(t *testing.T) {
	store := svc.NewAdapterConfigStore(closedDBForScan(t))
	repo := svc.NewAdapterConfigRepositoryService(store)

	_, err := repo.GetEffectiveScanConfig(context.Background(), "stripe")
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %v", err)
	}
}

func TestAdapterConfigRepo_UpdateScanIntervalOverride_OK(t *testing.T) {
	db := newAdapterConfigTestDB(t)
	store := svc.NewAdapterConfigStore(db)
	repo := svc.NewAdapterConfigRepositoryService(store)
	if err := db.Create(&companymodel.CompanyScanSettings{CompanyID: "stripe", AdapterType: "greenhouse", Active: true}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	v := 30
	if err := repo.UpdateScanIntervalOverride(context.Background(), "stripe", &v); err != nil {
		t.Fatalf("UpdateScanIntervalOverride() error = %v", err)
	}
}

func TestAdapterConfigRepo_UpdateScanIntervalOverride_NotFound(t *testing.T) {
	store := svc.NewAdapterConfigStore(newAdapterConfigTestDB(t))
	repo := svc.NewAdapterConfigRepositoryService(store)

	v := 30
	err := repo.UpdateScanIntervalOverride(context.Background(), "missing", &v)
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %v", err)
	}
}

func TestAdapterConfigRepo_UpdateScanIntervalOverride_DBError(t *testing.T) {
	store := svc.NewAdapterConfigStore(closedDBForScan(t))
	repo := svc.NewAdapterConfigRepositoryService(store)

	v := 30
	err := repo.UpdateScanIntervalOverride(context.Background(), "stripe", &v)
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %v", err)
	}
}
