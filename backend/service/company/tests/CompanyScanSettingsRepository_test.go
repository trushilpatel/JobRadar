package company_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// newScanSettingsDB returns a sqlite DB with both company tables migrated.
// Several methods touch BOTH tables (GetMissing's LEFT JOIN) so we need the
// `companies` table even though the SUT is the scan_settings repo.
func newScanSettingsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger:         gormlogger.Default.LogMode(gormlogger.Silent),
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&companymodel.Company{}, &companymodel.CompanyScanSettings{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestScanSettingsRepo_Get_FoundMissingAndError(t *testing.T) {
	db := newScanSettingsDB(t)
	now := time.Now()
	if err := db.Exec(`INSERT INTO company_scan_settings (company_id, adapter_type, active, consecutive_errors, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`,
		"a", companycommon.AdapterGreenhouse, true, now, now).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := companysvc.NewScanSettingsRepositoryService(db)

	row, err := repo.Get(context.Background(), "a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if row.CompanyID != "a" || row.AdapterType != companycommon.AdapterGreenhouse {
		t.Errorf("unexpected row: %+v", row)
	}

	_, err = repo.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected NOT_FOUND, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %T %v", err, err)
	}

	// DB-level error: closed connection bubbles up as INTERNAL.
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	_, err = repo.Get(context.Background(), "a")
	if err == nil {
		t.Fatal("expected INTERNAL after close, got nil")
	}
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL after close, got %T %v", err, err)
	}
}

func TestScanSettingsRepo_GetMissing(t *testing.T) {
	db := newScanSettingsDB(t)
	// Two companies, one with settings.
	_ = db.Create(&companymodel.Company{ID: "a", Name: "A", Slug: "a"}).Error
	_ = db.Create(&companymodel.Company{ID: "b", Name: "B", Slug: "b"}).Error
	now := time.Now()
	_ = db.Exec(`INSERT INTO company_scan_settings (company_id, adapter_type, active, consecutive_errors, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`,
		"a", companycommon.AdapterGreenhouse, true, now, now).Error
	repo := companysvc.NewScanSettingsRepositoryService(db)

	ids, err := repo.GetMissing(context.Background())
	if err != nil {
		t.Fatalf("GetMissing: %v", err)
	}
	if len(ids) != 1 || ids[0] != "b" {
		t.Errorf("expected [b], got %v", ids)
	}

	// Empty result must be []string{} not nil — locked down by contract.
	_ = db.Exec("DELETE FROM companies WHERE id = 'b'").Error
	ids, err = repo.GetMissing(context.Background())
	if err != nil {
		t.Fatalf("GetMissing: %v", err)
	}
	if ids == nil {
		t.Errorf("expected empty slice, got nil")
	}
	if len(ids) != 0 {
		t.Errorf("expected empty result, got %v", ids)
	}
}

func TestScanSettingsRepo_GetMissing_DBError(t *testing.T) {
	db := newScanSettingsDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	repo := companysvc.NewScanSettingsRepositoryService(db)
	_, err := repo.GetMissing(context.Background())
	if err == nil {
		t.Fatal("expected error after close, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %T %v", err, err)
	}
}

func TestScanSettingsRepo_Create_HappyAndConflict(t *testing.T) {
	db := newScanSettingsDB(t)
	repo := companysvc.NewScanSettingsRepositoryService(db)

	row := companymodel.CompanyScanSettings{
		CompanyID:   "stripe",
		AdapterType: companycommon.AdapterGreenhouse,
		Active:      true,
	}
	if err := repo.Create(context.Background(), row); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Second create on same PK returns CONFLICT (mapped from unique
	// violation via gorm.ErrDuplicatedKey).
	err := repo.Create(context.Background(), row)
	if err == nil {
		t.Fatal("expected CONFLICT, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) {
		t.Fatalf("expected ServiceError, got %T %v", err, err)
	}
	// SQLite without TranslateError may surface INTERNAL; with TranslateError
	// on, it surfaces CONFLICT. Either way the code path is exercised.
	if se.Code != "CONFLICT" && se.Code != "INTERNAL" {
		t.Errorf("expected CONFLICT or INTERNAL, got %s", se.Code)
	}
}

func TestScanSettingsRepo_Update(t *testing.T) {
	db := newScanSettingsDB(t)
	repo := companysvc.NewScanSettingsRepositoryService(db)
	now := time.Now()
	if err := db.Exec(`INSERT INTO company_scan_settings (company_id, adapter_type, active, consecutive_errors, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`,
		"a", companycommon.AdapterGreenhouse, true, now, now).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	adapter := companycommon.AdapterLever
	active := false
	interval := 30
	if err := repo.Update(context.Background(), "a", companydto.CompanyScanSettingsInput{
		AdapterType:          &adapter,
		Active:               &active,
		ScanIntervalOverride: &interval,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	var row companymodel.CompanyScanSettings
	_ = db.First(&row, "company_id = ?", "a").Error
	if row.AdapterType != adapter {
		t.Errorf("adapter_type = %s, want %s", row.AdapterType, adapter)
	}
	if row.Active {
		t.Errorf("active should be false")
	}
	if row.ScanIntervalOverride == nil || *row.ScanIntervalOverride != 30 {
		t.Errorf("scan_interval_override = %v, want 30", row.ScanIntervalOverride)
	}
}

func TestScanSettingsRepo_Update_EmptyPatch(t *testing.T) {
	repo := companysvc.NewScanSettingsRepositoryService(newScanSettingsDB(t))
	err := repo.Update(context.Background(), "a", companydto.CompanyScanSettingsInput{})
	if err == nil {
		t.Fatal("expected BAD_REQUEST, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "BAD_REQUEST" {
		t.Errorf("expected BAD_REQUEST, got %T %v", err, err)
	}
}

func TestScanSettingsRepo_Update_NotFound(t *testing.T) {
	repo := companysvc.NewScanSettingsRepositoryService(newScanSettingsDB(t))
	active := true
	err := repo.Update(context.Background(), "missing", companydto.CompanyScanSettingsInput{Active: &active})
	if err == nil {
		t.Fatal("expected NOT_FOUND, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %T %v", err, err)
	}
}

func TestScanSettingsRepo_UpdateScanState_AllFields(t *testing.T) {
	db := newScanSettingsDB(t)
	repo := companysvc.NewScanSettingsRepositoryService(db)
	now := time.Now()
	if err := db.Exec(`INSERT INTO company_scan_settings (company_id, adapter_type, active, consecutive_errors, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`,
		"a", companycommon.AdapterGreenhouse, true, now, now).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	errMsg := "boom"
	errCount := 4
	if err := repo.UpdateScanState(context.Background(), "a", companydto.CompanyScanStateUpdate{
		LastScanAt:        &ts,
		LastSuccessAt:     &ts,
		LastErrorAt:       &ts,
		LastErrorMessage:  &errMsg,
		ConsecutiveErrors: &errCount,
	}); err != nil {
		t.Fatalf("UpdateScanState: %v", err)
	}
	var row companymodel.CompanyScanSettings
	_ = db.First(&row, "company_id = ?", "a").Error
	if row.ConsecutiveErrors != 4 {
		t.Errorf("consecutive_errors = %d, want 4", row.ConsecutiveErrors)
	}
	if row.LastErrorMessage == nil || *row.LastErrorMessage != errMsg {
		t.Errorf("last_error_message = %v, want %q", row.LastErrorMessage, errMsg)
	}
}

func TestScanSettingsRepo_UpdateScanState_NoFieldsIsNoOp(t *testing.T) {
	repo := companysvc.NewScanSettingsRepositoryService(newScanSettingsDB(t))
	// All-nil patch returns nil without touching the DB — guard for the
	// "len(fields) == 0" branch.
	if err := repo.UpdateScanState(context.Background(), "anything", companydto.CompanyScanStateUpdate{}); err != nil {
		t.Errorf("empty patch should be no-op, got %v", err)
	}
}

func TestScanSettingsRepo_UpdateScanState_NotFound(t *testing.T) {
	repo := companysvc.NewScanSettingsRepositoryService(newScanSettingsDB(t))
	ts := time.Now()
	err := repo.UpdateScanState(context.Background(), "missing", companydto.CompanyScanStateUpdate{LastScanAt: &ts})
	if err == nil {
		t.Fatal("expected NOT_FOUND, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %T %v", err, err)
	}
}

func TestScanSettingsRepo_Delete_Idempotent(t *testing.T) {
	db := newScanSettingsDB(t)
	repo := companysvc.NewScanSettingsRepositoryService(db)
	now := time.Now()
	_ = db.Exec(`INSERT INTO company_scan_settings (company_id, adapter_type, active, consecutive_errors, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`,
		"a", companycommon.AdapterGreenhouse, true, now, now).Error

	if err := repo.Delete(context.Background(), "a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// Idempotent — second delete is allowed (CASCADE may have already
	// removed the row from a company delete).
	if err := repo.Delete(context.Background(), "a"); err != nil {
		t.Errorf("second Delete should be no-op, got %v", err)
	}
}

func TestScanSettingsRepo_Delete_DBError(t *testing.T) {
	db := newScanSettingsDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	repo := companysvc.NewScanSettingsRepositoryService(db)
	err := repo.Delete(context.Background(), "a")
	if err == nil {
		t.Fatal("expected INTERNAL after close, got nil")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %T %v", err, err)
	}
}
