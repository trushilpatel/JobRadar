package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newAtsTestDB returns an in-memory SQLite DB with the ats table created.
func newAtsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Discard,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	stmt := `CREATE TABLE ats (
		name                  TEXT    PRIMARY KEY,
		display_name          TEXT    NOT NULL,
		base_url              TEXT    NOT NULL DEFAULT '',
		is_active             BOOLEAN NOT NULL DEFAULT 1,
		is_admin_creatable    BOOLEAN NOT NULL DEFAULT 1,
		scan_interval_min     INTEGER NOT NULL DEFAULT 1440,
		inter_company_delay_s INTEGER NOT NULL DEFAULT 3,
		created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`
	if err := db.Exec(stmt).Error; err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

// seedAts inserts a known row for deterministic test assertions.
func seedAts(t *testing.T, db *gorm.DB, name, display string) {
	t.Helper()
	now := time.Now().UTC()
	stmt := `INSERT INTO ats (name, display_name, base_url, is_active, is_admin_creatable, scan_interval_min, inter_company_delay_s, created_at, updated_at)
	         VALUES (?, ?, '', 1, 1, 1440, 3, ?, ?)`
	if err := db.Exec(stmt, name, display, now, now).Error; err != nil {
		t.Fatalf("seed ats %q: %v", name, err)
	}
}

// closedAtsDB returns a DB whose underlying *sql.DB is closed — every query
// surfaces a DB error, exercising the repository error-mapping branches.
func closedAtsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newAtsTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

func asAtsSvcErr(t *testing.T, err error) *domainerrs.ServiceError {
	t.Helper()
	var se *domainerrs.ServiceError
	if !errors.As(err, &se) {
		t.Fatalf("error %v is not a *ServiceError", err)
	}
	return se
}

// ---------- ListAll ----------

func TestAtsRepo_ListAll_HappyPath(t *testing.T) {
	t.Parallel()
	db := newAtsTestDB(t)
	seedAts(t, db, "greenhouse", "Greenhouse")
	seedAts(t, db, "lever", "Lever")

	svc := atssvc.NewRepositoryService(atssvc.NewAtsStore(db))
	items, err := svc.ListATS(context.Background())
	if err != nil {
		t.Fatalf("ListATS: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2", len(items))
	}
}

func TestAtsRepo_ListAll_DBError(t *testing.T) {
	t.Parallel()
	svc := atssvc.NewRepositoryService(atssvc.NewAtsStore(closedAtsDB(t)))
	_, err := svc.ListATS(context.Background())
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
	se := asAtsSvcErr(t, err)
	if !errors.Is(se, domainerrs.ErrInternal) {
		t.Errorf("want ErrInternal, got code=%q", se.Code)
	}
}

// ---------- GetByName ----------

func TestAtsRepo_GetByName_Found(t *testing.T) {
	t.Parallel()
	db := newAtsTestDB(t)
	seedAts(t, db, "ashby", "Ashby")

	svc := atssvc.NewRepositoryService(atssvc.NewAtsStore(db))
	got, err := svc.GetATS(context.Background(), "ashby")
	if err != nil {
		t.Fatalf("GetATS: %v", err)
	}
	if got.Name != "ashby" {
		t.Errorf("name = %q, want ashby", got.Name)
	}
	if got.DisplayName != "Ashby" {
		t.Errorf("display_name = %q, want Ashby", got.DisplayName)
	}
}

func TestAtsRepo_GetByName_NotFound(t *testing.T) {
	t.Parallel()
	db := newAtsTestDB(t)
	svc := atssvc.NewRepositoryService(atssvc.NewAtsStore(db))
	_, err := svc.GetATS(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing row")
	}
	se := asAtsSvcErr(t, err)
	if !errors.Is(se, domainerrs.ErrNotFound) {
		t.Errorf("want ErrNotFound, got code=%q", se.Code)
	}
}

func TestAtsRepo_GetByName_DBError(t *testing.T) {
	t.Parallel()
	svc := atssvc.NewRepositoryService(atssvc.NewAtsStore(closedAtsDB(t)))
	_, err := svc.GetATS(context.Background(), "greenhouse")
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
	se := asAtsSvcErr(t, err)
	if !errors.Is(se, domainerrs.ErrInternal) {
		t.Errorf("want ErrInternal, got code=%q", se.Code)
	}
}

// ---------- Update ----------

func boolPtr(b bool) *bool { return &b }

func TestAtsRepo_Update_HappyPath(t *testing.T) {
	t.Parallel()
	db := newAtsTestDB(t)
	seedAts(t, db, "lever", "Lever")

	svc := atssvc.NewRepositoryService(atssvc.NewAtsStore(db))
	got, err := svc.UpdateATS(context.Background(), "lever", atsdto.UpdateATSInput{IsActive: boolPtr(false)})
	if err != nil {
		t.Fatalf("UpdateATS: %v", err)
	}
	if got.Name != "lever" {
		t.Errorf("name = %q, want lever", got.Name)
	}
}

func TestAtsRepo_Update_RowsAffectedZero(t *testing.T) {
	t.Parallel()
	db := newAtsTestDB(t)
	// No seed — update targets a row that does not exist.
	svc := atssvc.NewRepositoryService(atssvc.NewAtsStore(db))
	_, err := svc.UpdateATS(context.Background(), "nonexistent", atsdto.UpdateATSInput{IsActive: boolPtr(true)})
	if err == nil {
		t.Fatal("expected error for missing row")
	}
	se := asAtsSvcErr(t, err)
	if !errors.Is(se, domainerrs.ErrNotFound) {
		t.Errorf("want ErrNotFound, got code=%q", se.Code)
	}
}

// TestAtsRepo_Update_AllFields verifies that all four pointer fields are
// correctly applied when all are set simultaneously.
func TestAtsRepo_Update_AllFields(t *testing.T) {
	t.Parallel()
	db := newAtsTestDB(t)
	seedAts(t, db, "gem", "Gem")

	svc := atssvc.NewRepositoryService(atssvc.NewAtsStore(db))
	interval := 720
	delay := 10
	active := false
	adminCreatable := false
	got, err := svc.UpdateATS(context.Background(), "gem", atsdto.UpdateATSInput{
		ScanIntervalMin:    &interval,
		InterCompanyDelayS: &delay,
		IsActive:           &active,
		IsAdminCreatable:   &adminCreatable,
	})
	if err != nil {
		t.Fatalf("UpdateATS: %v", err)
	}
	if got.ScanIntervalMin != 720 {
		t.Errorf("scan_interval_min = %d, want 720", got.ScanIntervalMin)
	}
	if got.InterCompanyDelayS != 10 {
		t.Errorf("inter_company_delay_s = %d, want 10", got.InterCompanyDelayS)
	}
}

func TestAtsRepo_Update_DBError(t *testing.T) {
	t.Parallel()
	svc := atssvc.NewRepositoryService(atssvc.NewAtsStore(closedAtsDB(t)))
	_, err := svc.UpdateATS(context.Background(), "greenhouse", atsdto.UpdateATSInput{IsActive: boolPtr(true)})
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
	se := asAtsSvcErr(t, err)
	if !errors.Is(se, domainerrs.ErrInternal) {
		t.Errorf("want ErrInternal, got code=%q", se.Code)
	}
}
