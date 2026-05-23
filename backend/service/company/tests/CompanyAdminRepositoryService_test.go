package company_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// fakeScanHistory satisfies dto.ScanHistoryReader. Returns the canned map and
// records the IDs requested so tests can assert call shape when needed.
type fakeScanHistory struct {
	out map[string][]bool
	err error
}

func (f *fakeScanHistory) GetScanHistories(_ context.Context, _ []string) (map[string][]bool, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.out == nil {
		return map[string][]bool{}, nil
	}
	return f.out, nil
}

// newAdminTestDB returns an in-memory sqlite DB with the company tables
// migrated and a no-op scan-history reader. Most happy-path admin tests use
// this; the rest fall back to DryRun for branches with ILIKE / queue
// sub-SELECTs the sqlite driver can't run (the queue table is unmigrated).
func newAdminTestDB(t *testing.T) (*gorm.DB, *fakeScanHistory) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&companymodel.Company{}, &companymodel.CompanyScanSettings{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Stand-in for company_scan_queue so the fetchQueueStatusForCompanies
	// raw query has a table to read from. The queue table isn't a GORM
	// model in this package, so we declare the minimum schema by hand.
	if err := db.Exec(`CREATE TABLE company_scan_queue (
        id TEXT PRIMARY KEY,
        company_id TEXT NOT NULL,
        status TEXT NOT NULL
    )`).Error; err != nil {
		t.Fatalf("create queue table: %v", err)
	}
	return db, &fakeScanHistory{}
}

func seedAdminCompany(t *testing.T, db *gorm.DB, id, name string) {
	t.Helper()
	if err := db.Create(&companymodel.Company{ID: id, Name: name, Slug: strings.ToLower(name)}).Error; err != nil {
		t.Fatalf("seed company: %v", err)
	}
}

func seedScanSettings(t *testing.T, db *gorm.DB, companyID, adapter string, active bool) {
	t.Helper()
	// GORM Create with a zero-value bool falls back to the column default
	// (`default:true`), which would defeat the active=false branch under
	// test. Seeding via raw SQL writes the exact value supplied.
	now := time.Now()
	if err := db.Exec(`INSERT INTO company_scan_settings
        (company_id, adapter_type, active, consecutive_errors, created_at, updated_at)
        VALUES (?, ?, ?, 0, ?, ?)`,
		companyID, adapter, active, now, now).Error; err != nil {
		t.Fatalf("seed scan_settings: %v", err)
	}
}

func TestAdminRepo_Get_FoundWithoutSettings(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "stripe", "Stripe")
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	view, err := repo.Get(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if view == nil || view.ID != "stripe" {
		t.Fatalf("unexpected view: %+v", view)
	}
	if view.ScanSettings != nil {
		t.Errorf("ScanSettings should be nil for company without settings, got %+v", view.ScanSettings)
	}
}

func TestAdminRepo_Get_FoundWithSettings(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "stripe", "Stripe")
	seedScanSettings(t, db, "stripe", companycommon.AdapterGreenhouse, true)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	view, err := repo.Get(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if view.ScanSettings == nil {
		t.Fatalf("expected ScanSettings populated, got nil")
	}
	if view.ScanSettings.AdapterType != companycommon.AdapterGreenhouse {
		t.Errorf("adapter_type = %q, want greenhouse", view.ScanSettings.AdapterType)
	}
	if !view.ScanSettings.Active {
		t.Errorf("active should be true, got false")
	}
}

func TestAdminRepo_Get_NotFound(t *testing.T) {
	db, hist := newAdminTestDB(t)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	_, err := repo.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected NOT_FOUND, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND ServiceError, got %T %v", err, err)
	}
}

func TestAdminRepo_Create_NoSettings(t *testing.T) {
	db, hist := newAdminTestDB(t)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	web := "https://stripe.com"
	careers := "https://stripe.com/jobs"
	logo := "https://stripe.com/logo.png"
	view, err := repo.Create(context.Background(),
		companydto.CompanyPublicInput{
			ID: "stripe", Name: "Stripe", Slug: "stripe",
			Website: &web, CareersURL: &careers, LogoURL: &logo,
		},
		companydto.CompanyScanSettingsInput{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if view.ID != "stripe" || view.Website != web {
		t.Errorf("view fields wrong: %+v", view)
	}
	if view.ScanSettings != nil {
		t.Errorf("ScanSettings should be nil when settings input empty, got %+v", view.ScanSettings)
	}
	// Verify the row landed in the DB.
	var n int64
	_ = db.Model(&companymodel.Company{}).Where("id = ?", "stripe").Count(&n).Error
	if n != 1 {
		t.Errorf("company row count = %d, want 1", n)
	}
}

func TestAdminRepo_Create_WithSettings(t *testing.T) {
	db, hist := newAdminTestDB(t)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	adapter := companycommon.AdapterGreenhouse
	cfg := datatypes.JSON([]byte(`{"board":"stripe"}`))
	interval := 60
	active := true
	view, err := repo.Create(context.Background(),
		companydto.CompanyPublicInput{ID: "stripe", Name: "Stripe", Slug: "stripe"},
		companydto.CompanyScanSettingsInput{
			AdapterType:          &adapter,
			AdapterConfig:        &cfg,
			ScanIntervalOverride: &interval,
			Active:               &active,
		})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if view.ScanSettings == nil {
		t.Fatalf("expected scan_settings populated")
	}
	if view.ScanSettings.AdapterType != adapter {
		t.Errorf("adapter_type = %q, want %q", view.ScanSettings.AdapterType, adapter)
	}
	if view.ScanSettings.ScanIntervalOverride == nil || *view.ScanSettings.ScanIntervalOverride != 60 {
		t.Errorf("scan_interval_override = %v, want 60", view.ScanSettings.ScanIntervalOverride)
	}
}

func TestAdminRepo_Create_DuplicateID(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "stripe", "Stripe")
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	_, err := repo.Create(context.Background(),
		companydto.CompanyPublicInput{ID: "stripe", Name: "Dup", Slug: "dup"},
		companydto.CompanyScanSettingsInput{})
	if err == nil {
		t.Fatal("expected error on duplicate id, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL error, got %T %v", err, err)
	}
}

func TestAdminRepo_UpdatePublic_Patch(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "stripe", "Stripe")
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	web := "https://stripe-new.com"
	if err := repo.UpdatePublic(context.Background(), "stripe", companydto.CompanyPublicInput{Name: "Renamed", Website: &web}); err != nil {
		t.Fatalf("UpdatePublic: %v", err)
	}
	var got companymodel.Company
	if err := db.First(&got, "id = ?", "stripe").Error; err != nil {
		t.Fatalf("readback: %v", err)
	}
	if got.Name != "Renamed" || got.Website != web {
		t.Errorf("patch didn't land: %+v", got)
	}
}

func TestAdminRepo_UpdatePublic_NoFields(t *testing.T) {
	db, hist := newAdminTestDB(t)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	err := repo.UpdatePublic(context.Background(), "anything", companydto.CompanyPublicInput{})
	if err == nil {
		t.Fatal("expected BAD_REQUEST, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "BAD_REQUEST" {
		t.Errorf("expected BAD_REQUEST, got %T %v", err, err)
	}
}

func TestAdminRepo_UpdatePublic_NotFound(t *testing.T) {
	db, hist := newAdminTestDB(t)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	err := repo.UpdatePublic(context.Background(), "missing", companydto.CompanyPublicInput{Name: "x"})
	if err == nil {
		t.Fatal("expected NOT_FOUND, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %T %v", err, err)
	}
}

func TestAdminRepo_UpdateScanSettings_Patch(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "stripe", "Stripe")
	seedScanSettings(t, db, "stripe", companycommon.AdapterGreenhouse, true)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	adapter := companycommon.AdapterLever
	active := false
	if err := repo.UpdateScanSettings(context.Background(), "stripe", companydto.CompanyScanSettingsInput{AdapterType: &adapter, Active: &active}); err != nil {
		t.Fatalf("UpdateScanSettings: %v", err)
	}
	var got companymodel.CompanyScanSettings
	if err := db.First(&got, "company_id = ?", "stripe").Error; err != nil {
		t.Fatalf("readback: %v", err)
	}
	if got.AdapterType != adapter || got.Active {
		t.Errorf("patch didn't land: %+v", got)
	}
}

func TestAdminRepo_UpdateScanSettings_Empty(t *testing.T) {
	db, hist := newAdminTestDB(t)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	err := repo.UpdateScanSettings(context.Background(), "stripe", companydto.CompanyScanSettingsInput{})
	if err == nil {
		t.Fatal("expected BAD_REQUEST, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "BAD_REQUEST" {
		t.Errorf("expected BAD_REQUEST, got %T %v", err, err)
	}
}

func TestAdminRepo_UpdateScanSettings_NotFound(t *testing.T) {
	db, hist := newAdminTestDB(t)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	active := false
	err := repo.UpdateScanSettings(context.Background(), "missing", companydto.CompanyScanSettingsInput{Active: &active})
	if err == nil {
		t.Fatal("expected NOT_FOUND, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %T %v", err, err)
	}
}

func TestAdminRepo_Delete_FoundAndMissing(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "stripe", "Stripe")
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	if err := repo.Delete(context.Background(), "stripe"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var n int64
	_ = db.Model(&companymodel.Company{}).Count(&n).Error
	if n != 0 {
		t.Errorf("expected row deleted, count = %d", n)
	}

	// Second delete must surface NOT_FOUND.
	err := repo.Delete(context.Background(), "stripe")
	if err == nil {
		t.Fatal("expected NOT_FOUND, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %T %v", err, err)
	}
}

func TestAdminRepo_SetAllActive(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "A")
	seedAdminCompany(t, db, "b", "B")
	seedScanSettings(t, db, "a", companycommon.AdapterGreenhouse, true)
	seedScanSettings(t, db, "b", companycommon.AdapterLever, true)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	if err := repo.SetAllActive(context.Background(), false); err != nil {
		t.Fatalf("SetAllActive: %v", err)
	}
	var rows []companymodel.CompanyScanSettings
	_ = db.Find(&rows).Error
	for _, r := range rows {
		if r.Active {
			t.Errorf("row %s still active", r.CompanyID)
		}
	}

	// Empty table is allowed — must not error.
	_ = db.Exec("DELETE FROM company_scan_settings").Error
	if err := repo.SetAllActive(context.Background(), true); err != nil {
		t.Errorf("SetAllActive on empty: %v", err)
	}
}

func TestAdminRepo_GetByIDs_EmptyAndPopulated(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "A")
	seedAdminCompany(t, db, "b", "B")
	seedScanSettings(t, db, "a", companycommon.AdapterGreenhouse, true)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	// Empty input returns empty slice (not nil) — locked down by contract.
	views, err := repo.GetByIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetByIDs nil: %v", err)
	}
	if views == nil {
		t.Errorf("expected empty slice, got nil")
	}
	if len(views) != 0 {
		t.Errorf("expected empty slice, got %d", len(views))
	}

	views, err = repo.GetByIDs(context.Background(), []string{"a", "b", "missing"})
	if err != nil {
		t.Fatalf("GetByIDs: %v", err)
	}
	if len(views) != 2 {
		t.Errorf("expected 2 views, got %d", len(views))
	}
	// At least one view should have settings populated.
	var withSettings int
	for _, v := range views {
		if v.ScanSettings != nil {
			withSettings++
		}
	}
	if withSettings != 1 {
		t.Errorf("expected 1 view with settings, got %d", withSettings)
	}
}

func TestAdminRepo_GetAll(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "b", "B")
	seedAdminCompany(t, db, "a", "A")
	seedScanSettings(t, db, "a", companycommon.AdapterGreenhouse, true)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	views, err := repo.GetAll(context.Background())
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(views) != 2 {
		t.Errorf("expected 2, got %d", len(views))
	}
	// Sorted ASC by name → A then B.
	if views[0].Name != "A" {
		t.Errorf("first = %s, want A", views[0].Name)
	}
}

func TestAdminRepo_List_NoFilter(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "Alpha")
	seedAdminCompany(t, db, "b", "Bravo")
	seedScanSettings(t, db, "a", companycommon.AdapterGreenhouse, true)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	res, err := repo.List(context.Background(), companydto.CompanyFilter{}, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if res.Total != 2 {
		t.Errorf("Total = %d, want 2", res.Total)
	}
	if res.TotalActive != 1 {
		t.Errorf("TotalActive = %d, want 1", res.TotalActive)
	}
}

func TestAdminRepo_List_ActiveFilter(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "Alpha")
	seedAdminCompany(t, db, "b", "Bravo")
	seedAdminCompany(t, db, "c", "Charlie")
	seedScanSettings(t, db, "a", companycommon.AdapterGreenhouse, true)
	seedScanSettings(t, db, "b", companycommon.AdapterLever, false)
	// c has no scan_settings row.
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	// Active=true matches only company a.
	active := true
	res, err := repo.List(context.Background(), companydto.CompanyFilter{Active: &active}, 10)
	if err != nil {
		t.Fatalf("List active=true: %v", err)
	}
	if res.Total != 1 || len(res.Items) != 1 || res.Items[0].ID != "a" {
		t.Errorf("active=true list wrong: total=%d items=%v", res.Total, idsOf(res.Items))
	}

	// Active=false matches b AND c (no settings row counts as inactive).
	active = false
	res, err = repo.List(context.Background(), companydto.CompanyFilter{Active: &active}, 10)
	if err != nil {
		t.Fatalf("List active=false: %v", err)
	}
	if res.Total != 2 {
		t.Errorf("active=false total = %d, want 2 (b + c)", res.Total)
	}
}

func TestAdminRepo_List_HasSettingsFilter(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "Alpha")
	seedAdminCompany(t, db, "b", "Bravo")
	seedScanSettings(t, db, "a", companycommon.AdapterGreenhouse, true)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	hs := true
	res, err := repo.List(context.Background(), companydto.CompanyFilter{HasSettings: &hs}, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if res.Total != 1 || res.Items[0].ID != "a" {
		t.Errorf("has_settings=true wrong: %+v", idsOf(res.Items))
	}

	hs = false
	res, err = repo.List(context.Background(), companydto.CompanyFilter{HasSettings: &hs}, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if res.Total != 1 || res.Items[0].ID != "b" {
		t.Errorf("has_settings=false wrong: %+v", idsOf(res.Items))
	}
}

func TestAdminRepo_List_AdapterTypesFilter(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "Alpha")
	seedAdminCompany(t, db, "b", "Bravo")
	seedScanSettings(t, db, "a", companycommon.AdapterGreenhouse, true)
	seedScanSettings(t, db, "b", companycommon.AdapterLever, true)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	res, err := repo.List(context.Background(),
		companydto.CompanyFilter{AdapterTypes: []string{companycommon.AdapterGreenhouse}}, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if res.Total != 1 || res.Items[0].ID != "a" {
		t.Errorf("adapter filter wrong: %+v", idsOf(res.Items))
	}
}

func TestAdminRepo_List_ScanStatusBranches(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "Alpha")
	seedAdminCompany(t, db, "b", "Bravo")
	// a: ok (last_scan_at set, no errors)
	now := time.Now()
	if err := db.Create(&companymodel.CompanyScanSettings{
		CompanyID:         "a",
		AdapterType:       companycommon.AdapterGreenhouse,
		Active:            true,
		LastScanAt:        &now,
		ConsecutiveErrors: 0,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	// b: error (consecutive_errors > 0)
	if err := db.Create(&companymodel.CompanyScanSettings{
		CompanyID:         "b",
		AdapterType:       companycommon.AdapterLever,
		Active:            true,
		ConsecutiveErrors: 3,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Seed a queued row in the scan queue.
	if err := db.Exec(`INSERT INTO company_scan_queue (id, company_id, status) VALUES (?, ?, ?)`, "q1", "a", "queued").Error; err != nil {
		t.Fatalf("seed queue: %v", err)
	}
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	cases := []struct {
		status  string
		wantIDs []string
	}{
		{"ok", []string{"a"}},
		{"error", []string{"b"}},
		{"queued", []string{"a"}},
		{"running", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			res, err := repo.List(context.Background(), companydto.CompanyFilter{ScanStatus: tc.status}, 10)
			if err != nil {
				t.Fatalf("List %s: %v", tc.status, err)
			}
			got := idsOf(res.Items)
			if !sameSet(got, tc.wantIDs) {
				t.Errorf("status=%s: got %v, want %v", tc.status, got, tc.wantIDs)
			}
		})
	}
}

func TestAdminRepo_List_SortBranches(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "Bravo")
	seedAdminCompany(t, db, "b", "Alpha")
	seedScanSettings(t, db, "a", companycommon.AdapterGreenhouse, true)
	seedScanSettings(t, db, "b", companycommon.AdapterLever, true)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	cases := []struct {
		sortBy  string
		sortDir string
		wantID  string
	}{
		{"adapter_type", "asc", "a"},  // greenhouse < lever
		{"adapter_type", "desc", "b"}, // lever first
		{"last_scan_at", "asc", ""},   // both NULL — order indeterminate but query must run
		{"created_at", "asc", ""},
		{"", "", "b"}, // default: c.name ASC → Alpha first → id=b
	}
	for _, tc := range cases {
		t.Run(tc.sortBy+"-"+tc.sortDir, func(t *testing.T) {
			res, err := repo.List(context.Background(), companydto.CompanyFilter{SortBy: tc.sortBy, SortDir: tc.sortDir}, 10)
			if err != nil {
				t.Fatalf("List sort=%s/%s: %v", tc.sortBy, tc.sortDir, err)
			}
			if tc.wantID != "" && res.Items[0].ID != tc.wantID {
				t.Errorf("first = %s, want %s", res.Items[0].ID, tc.wantID)
			}
		})
	}
}

func TestAdminRepo_List_QueueStatusPopulated(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "Alpha")
	if err := db.Exec(`INSERT INTO company_scan_queue (id, company_id, status) VALUES (?, ?, ?)`, "q1", "a", "running").Error; err != nil {
		t.Fatalf("seed queue: %v", err)
	}
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	res, err := repo.List(context.Background(), companydto.CompanyFilter{}, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].QueueStatus == nil || *res.Items[0].QueueStatus != "running" {
		t.Errorf("expected queue_status=running, got %+v", res.Items[0].QueueStatus)
	}
}

func TestAdminRepo_List_ScanHistoryPopulated(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "Alpha")
	hist.out = map[string][]bool{"a": {true, false, true}}
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	res, err := repo.List(context.Background(), companydto.CompanyFilter{}, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(res.Items[0].ScanHistory) != 3 {
		t.Errorf("expected 3 history entries, got %v", res.Items[0].ScanHistory)
	}
}

func TestAdminRepo_List_ScanHistoryErrorBubbles(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "Alpha")
	hist.err = errors.New("boom")
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	_, err := repo.List(context.Background(), companydto.CompanyFilter{}, 10)
	if err == nil {
		t.Fatal("expected error from scan-history reader, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %T %v", err, err)
	}
}

// idsOf returns the IDs of admin list views for set comparison.
func idsOf(items []companydto.CompanyAdminListView) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

// sameSet reports whether a and b contain the same ids (order-independent).
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]int, len(a))
	for _, x := range a {
		m[x]++
	}
	for _, x := range b {
		m[x]--
		if m[x] < 0 {
			return false
		}
	}
	return true
}

// TestAdminRepo_List_SearchFilter verifies that the Search field adds an ILIKE
// clause that is emitted in the SQL. Because SQLite does not support ILIKE we
// use DryRun mode and inspect the captured statements — the same pattern as
// TestPublicRepo_ListPublic_SearchEmitsILIKE in CompanyRepositoryService_test.go.
func TestAdminRepo_List_SearchFilter(t *testing.T) {
	cap := &publicSQLCapture{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DryRun: true, Logger: cap})
	if err != nil {
		t.Fatalf("open dryrun: %v", err)
	}
	hist := &fakeScanHistory{}
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	_, _ = repo.List(context.Background(), companydto.CompanyFilter{Search: "alpha"}, 10)

	captured := cap.statements()
	if len(captured) == 0 {
		t.Fatal("no SQL captured")
	}
	foundILIKE := false
	for _, s := range captured {
		if strings.Contains(s, "ILIKE") {
			foundILIKE = true
			break
		}
	}
	if !foundILIKE {
		t.Errorf("expected ILIKE in emitted SQL, got: %v", captured)
	}
}

// TestAdminRepo_Get_ScanSettingsDBError verifies that when the company row
// exists but the scan_settings table query fails with a non-not-found error
// (e.g. table dropped), Get returns ErrInternal, not ErrNotFound.
func TestAdminRepo_Get_ScanSettingsDBError(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "stripe", "Stripe")

	// Drop the scan_settings table to cause a hard DB error on the second
	// query inside Get(). The company row still exists so the first query
	// succeeds; only the settings query fails — exercising the
	// "non-not-found error → ErrInternal" branch.
	if err := db.Exec("DROP TABLE company_scan_settings").Error; err != nil {
		t.Fatalf("drop table: %v", err)
	}

	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	_, err := repo.Get(context.Background(), "stripe")
	if err == nil {
		t.Fatal("expected INTERNAL error, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL ServiceError, got %T %v", err, err)
	}
}
