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
	"github.com/trushilpatel/jobradar/common/logger"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// failingCache implements cache.Cache and returns a transport error on every
// operation. It drives the best-effort cache-error log branches in the admin
// impl decorator and the cache decorator (P0.3 paths) without a real Redis.
type failingCache struct{ err error }

func (f *failingCache) Get(_ context.Context, _ string, _ any) (bool, error) {
	return false, f.err
}
func (f *failingCache) Set(_ context.Context, _ string, _ any, _ time.Duration) error { return f.err }
func (f *failingCache) Delete(_ context.Context, _ ...string) error                   { return f.err }

// TestAdminImpl_CacheErrorsAreNonFatal verifies P0.3: cache transport failures
// on every mutating path are logged but never surface to the caller — the DB
// write already succeeded.
func TestAdminImpl_CacheErrorsAreNonFatal(t *testing.T) {
	fc := &failingCache{err: errors.New("redis down")}
	fake := &fakeAdmin{
		createView: &companydto.CompanyAdminView{CompanyAdminCommon: companydto.CompanyAdminCommon{Company: companymodel.Company{ID: "x"}}},
	}
	svc := companysvc.NewAdminImplService(fake, fc, logger.NewNoop())

	if _, err := svc.Create(context.Background(), companydto.CompanyPublicInput{ID: "x"}, companydto.CompanyScanSettingsInput{}); err != nil {
		t.Errorf("Create must not surface cache error, got %v", err)
	}
	if err := svc.UpdatePublic(context.Background(), "x", companydto.CompanyPublicInput{Name: "Y"}); err != nil {
		t.Errorf("UpdatePublic must not surface cache error, got %v", err)
	}
	if err := svc.Delete(context.Background(), "x"); err != nil {
		t.Errorf("Delete must not surface cache error, got %v", err)
	}
}

// TestAdminImpl_DelegationErrorsPropagate verifies the admin impl returns the
// underlying error from each mutating method without masking it.
func TestAdminImpl_DelegationErrorsPropagate(t *testing.T) {
	boom := apperr.ErrInternal.WithMessage("boom")
	fake := &fakeAdmin{createErr: boom, updateErr: boom, deleteErr: boom}
	svc := companysvc.NewAdminImplService(fake, nil, logger.NewNoop())

	if _, err := svc.Create(context.Background(), companydto.CompanyPublicInput{ID: "x"}, companydto.CompanyScanSettingsInput{}); err == nil {
		t.Error("Create error must propagate")
	}
	if err := svc.UpdatePublic(context.Background(), "x", companydto.CompanyPublicInput{Name: "Y"}); err == nil {
		t.Error("UpdatePublic error must propagate")
	}
	if err := svc.Delete(context.Background(), "x"); err == nil {
		t.Error("Delete error must propagate")
	}
}

// TestCache_ReadListGen_TransportError verifies P0.3: a cache.Get transport
// error while reading the list generation degrades to generation 0 and still
// reaches the source of truth (no error surfaced).
func TestCache_ReadListGen_TransportError(t *testing.T) {
	fc := &failingCache{err: errors.New("redis down")}
	fake := &fakePublic{listResult: companydto.CompanyPublicListResult{Total: 1}}
	svc := companysvc.NewCacheService(fake, fc, time.Minute, logger.NewNoop())

	if _, err := svc.ListPublic(context.Background(), companydto.CompanyFilter{}, 10); err != nil {
		t.Fatalf("ListPublic must degrade on cache error, got %v", err)
	}
	if fake.listCalls != 1 {
		t.Errorf("expected fall-through to source on cache error, calls=%d", fake.listCalls)
	}
}

// TestCache_GetPublic_TransportError covers the GetPublic cache-error log
// branch — a transport error is treated as a miss.
func TestCache_GetPublic_TransportError(t *testing.T) {
	fc := &failingCache{err: errors.New("redis down")}
	view := &companydto.CompanyPublicView{Company: companymodel.Company{ID: "x"}}
	fake := &fakePublic{getResult: view}
	svc := companysvc.NewCacheService(fake, fc, time.Minute, logger.NewNoop())

	if _, err := svc.GetPublic(context.Background(), "x"); err != nil {
		t.Fatalf("GetPublic must degrade on cache error, got %v", err)
	}
	if fake.getCalls != 1 {
		t.Errorf("expected fall-through to source, calls=%d", fake.getCalls)
	}
}

// newFKDB builds a sqlite DB with companies + ats + a scan_settings table
// whose adapter_type FKs into ats, mirroring migration 077.
func newFKDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?_foreign_keys=on"), &gorm.Config{
		Logger:         gormlogger.Default.LogMode(gormlogger.Silent),
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	stmts := []string{
		`CREATE TABLE companies (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, slug TEXT NOT NULL UNIQUE,
			website TEXT, careers_url TEXT, logo_url TEXT,
			created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE ats (name TEXT PRIMARY KEY)`,
		`CREATE TABLE company_scan_settings (
			company_id TEXT PRIMARY KEY,
			adapter_type TEXT NOT NULL REFERENCES ats(name),
			adapter_config TEXT, scan_interval_override INTEGER,
			active INTEGER NOT NULL DEFAULT 1,
			last_scan_at DATETIME, last_success_at DATETIME, last_error_at DATETIME,
			last_error_message TEXT, consecutive_errors INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME, updated_at DATETIME,
			FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE)`,
		`INSERT INTO ats (name) VALUES ('greenhouse')`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return db
}

// TestScanSettingsRepo_Create_ConflictAndError covers the ScanSettings repo
// Create conflict branch (P1.5/P1.1 shared helper) and a hard DB error.
func TestScanSettingsRepo_Create_ConflictAndError(t *testing.T) {
	db := newFKDB(t)
	if err := db.Exec(`INSERT INTO companies (id, name, slug) VALUES ('a','A','a')`).Error; err != nil {
		t.Fatalf("seed company: %v", err)
	}
	repo := companysvc.NewScanSettingsRepositoryService(db)

	row := companymodel.CompanyScanSettings{CompanyID: "a", AdapterType: companycommon.AdapterGreenhouse, Active: true}
	if err := repo.Create(context.Background(), row); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	// Duplicate PK → CONFLICT.
	err := repo.Create(context.Background(), row)
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "CONFLICT" {
		t.Errorf("expected CONFLICT on duplicate, got %T %v", err, err)
	}

	// Hard DB error (table dropped) → INTERNAL.
	if err := db.Exec("DROP TABLE company_scan_settings").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	err = repo.Create(context.Background(), companymodel.CompanyScanSettings{CompanyID: "b", AdapterType: companycommon.AdapterGreenhouse})
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL on dropped table, got %T %v", err, err)
	}
}

// TestScanSettingsRepo_Update_DBError covers the Update hard-error log branch.
// A closed DB handle guarantees a transport error (a dropped table on sqlite
// can surface as 0-rows → NOT_FOUND, which exercises a different branch).
func TestScanSettingsRepo_Update_DBError(t *testing.T) {
	db, _ := newAdminTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	repo := companysvc.NewScanSettingsRepositoryService(db)
	active := false
	err := repo.Update(context.Background(), "a", companydto.CompanyScanSettingsInput{Active: &active})
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %T %v", err, err)
	}
}

// TestAdminRepo_UpdateScanSettings_DBError covers the admin repo
// UpdateScanSettings hard-error log branch (adapter_type now in the log line).
func TestAdminRepo_UpdateScanSettings_DBError(t *testing.T) {
	db, hist := newAdminTestDB(t)
	if err := db.Exec("DROP TABLE company_scan_settings").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	adapter := companycommon.AdapterLever
	err := repo.UpdateScanSettings(context.Background(), "a", companydto.CompanyScanSettingsInput{AdapterType: &adapter})
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %T %v", err, err)
	}
}

// TestAdminRepo_UpdatePublic_DBError covers the admin repo UpdatePublic
// hard-error branch.
func TestAdminRepo_UpdatePublic_DBError(t *testing.T) {
	db, hist := newAdminTestDB(t)
	if err := db.Exec("DROP TABLE companies").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	web := "https://x.com"
	err := repo.UpdatePublic(context.Background(), "a", companydto.CompanyPublicInput{Website: &web})
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %T %v", err, err)
	}
}

// TestAdminRepo_Delete_DBError covers the admin repo Delete hard-error branch.
func TestAdminRepo_Delete_DBError(t *testing.T) {
	db, hist := newAdminTestDB(t)
	if err := db.Exec("DROP TABLE companies").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	err := repo.Delete(context.Background(), "a")
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %T %v", err, err)
	}
}

// TestAdminRepo_GetByIDs_DBError covers the GetByIDs companies-query error.
func TestAdminRepo_GetByIDs_DBError(t *testing.T) {
	db, hist := newAdminTestDB(t)
	if err := db.Exec("DROP TABLE companies").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	_, err := repo.GetByIDs(context.Background(), []string{"a"})
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %T %v", err, err)
	}
}

// TestAdminRepo_GetAll_DBError covers the GetAll companies-query error.
func TestAdminRepo_GetAll_DBError(t *testing.T) {
	db, hist := newAdminTestDB(t)
	if err := db.Exec("DROP TABLE companies").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	_, err := repo.GetAll(context.Background())
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %T %v", err, err)
	}
}

// TestAdminRepo_FetchSettings_DBError covers fetchSettingsForCompanies error
// path via List when the scan_settings table is dropped after the companies
// query succeeds.
func TestAdminRepo_List_SettingsFetchError(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "Alpha")
	if err := db.Exec("DROP TABLE company_scan_settings").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	_, err := repo.List(context.Background(), companydto.CompanyFilter{}, 10)
	if err == nil {
		t.Fatal("expected error when scan_settings table missing")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %T %v", err, err)
	}
}

// TestAdminRepo_List_QueueFetchError covers fetchQueueStatusForCompanies error.
// The queue table is dropped AND replaced with an incompatible shape so the
// raw SELECT fails (a plain drop on sqlite can yield an empty result instead
// of an error).
func TestAdminRepo_List_QueueFetchError(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "Alpha")
	if err := db.Exec("DROP TABLE company_scan_queue").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	// Recreate without the `status` column the raw query selects → SQL error.
	if err := db.Exec(`CREATE TABLE company_scan_queue (id TEXT PRIMARY KEY, company_id TEXT NOT NULL)`).Error; err != nil {
		t.Fatalf("recreate: %v", err)
	}
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	_, err := repo.List(context.Background(), companydto.CompanyFilter{}, 10)
	if err == nil {
		t.Fatal("expected error when queue table shape is wrong")
	}
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %T %v", err, err)
	}
}

// TestAdminValidator_FieldLengthCaps covers the UpdatePublic and
// validatePublicCreate max-length branches (P2 plumbing kept intact).
func TestAdminValidator_FieldLengthCaps(t *testing.T) {
	v := newAdminValidator()
	long := stringOfLen(300)
	longURL := stringOfLen(3000)

	// UpdatePublic over-length branches.
	if err := v.UpdatePublic(context.Background(), "id", companydto.CompanyPublicInput{Name: long}); err == nil {
		t.Error("over-length name should reject")
	}
	if err := v.UpdatePublic(context.Background(), "id", companydto.CompanyPublicInput{Slug: stringOfLen(200)}); err == nil {
		t.Error("over-length slug should reject")
	}
	if err := v.UpdatePublic(context.Background(), "id", companydto.CompanyPublicInput{Website: &longURL}); err == nil {
		t.Error("over-length website should reject")
	}
	if err := v.UpdatePublic(context.Background(), "id", companydto.CompanyPublicInput{CareersURL: &longURL}); err == nil {
		t.Error("over-length careers_url should reject")
	}
	if err := v.UpdatePublic(context.Background(), "id", companydto.CompanyPublicInput{LogoURL: &longURL}); err == nil {
		t.Error("over-length logo_url should reject")
	}

	// Create over-length branches.
	mk := func(p companydto.CompanyPublicInput) error {
		_, err := v.Create(context.Background(), p, companydto.CompanyScanSettingsInput{})
		return err
	}
	if err := mk(companydto.CompanyPublicInput{ID: stringOfLen(200), Name: "N", Slug: "s"}); err == nil {
		t.Error("over-length id should reject")
	}
	if err := mk(companydto.CompanyPublicInput{ID: "i", Name: long, Slug: "s"}); err == nil {
		t.Error("over-length create name should reject")
	}
	if err := mk(companydto.CompanyPublicInput{ID: "i", Name: "N", Slug: stringOfLen(200)}); err == nil {
		t.Error("over-length create slug should reject")
	}
	if err := mk(companydto.CompanyPublicInput{ID: "i", Name: "N", Slug: "s", Website: &longURL}); err == nil {
		t.Error("over-length create website should reject")
	}
	if err := mk(companydto.CompanyPublicInput{ID: "i", Name: "N", Slug: "s", CareersURL: &longURL}); err == nil {
		t.Error("over-length create careers_url should reject")
	}
	if err := mk(companydto.CompanyPublicInput{ID: "i", Name: "N", Slug: "s", LogoURL: &longURL}); err == nil {
		t.Error("over-length create logo_url should reject")
	}
}

// TestAdminValidator_ScanSettings_AdapterConfigCap covers the adapter_config
// size cap branch in validateScanSettings.
func TestAdminValidator_ScanSettings_AdapterConfigCap(t *testing.T) {
	v := newAdminValidator()
	big := jsonPtr(`{"k":"` + stringOfLen(11000) + `"}`)
	err := v.UpdateScanSettings(context.Background(), "id", companydto.CompanyScanSettingsInput{AdapterConfig: big})
	if err == nil {
		t.Error("over-size adapter_config should reject")
	}
}

// TestAdminRepo_Create_PublicFieldsPlumbed covers the careers_url / logo_url
// pointer-copy branches in admin Create.
func TestAdminRepo_Create_PublicFieldsPlumbed(t *testing.T) {
	db, hist := newAdminTestDB(t)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	careers := "https://acme.com/jobs"
	logo := "https://acme.com/logo.png"
	view, err := repo.Create(context.Background(),
		companydto.CompanyPublicInput{ID: "acme", Name: "Acme", Slug: "acme", CareersURL: &careers, LogoURL: &logo},
		companydto.CompanyScanSettingsInput{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if view.CareersURL == nil || *view.CareersURL != careers {
		t.Errorf("careers_url not plumbed: %v", view.CareersURL)
	}
	if view.LogoURL == nil || *view.LogoURL != logo {
		t.Errorf("logo_url not plumbed: %v", view.LogoURL)
	}
}

// TestScanSettingsRepo_Update_WithAdapter_DBError covers adapterTypeFromInput's
// non-nil branch: an Update that patches adapter_type and then fails (closed
// DB) logs the adapter_type, exercising the helper's populated path.
func TestScanSettingsRepo_Update_WithAdapter_DBError(t *testing.T) {
	db, _ := newAdminTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	repo := companysvc.NewScanSettingsRepositoryService(db)
	adapter := companycommon.AdapterLever
	err := repo.Update(context.Background(), "a", companydto.CompanyScanSettingsInput{AdapterType: &adapter})
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %T %v", err, err)
	}
}

// TestAdminRepo_Create_FKError_WithAdapterLogged covers adapterTypeOf's
// non-empty branch: a FK-violation Create with an adapter_type present logs the
// adapter and returns BAD_REQUEST.
func TestAdminRepo_Create_FKError_WithAdapterLogged(t *testing.T) {
	db := newFKDB(t)
	repo := companysvc.NewAdminRepositoryService(db, &fakeScanHistory{}, logger.NewNoop())
	bogus := "unregistered"
	_, err := repo.Create(context.Background(),
		companydto.CompanyPublicInput{ID: "acme", Name: "Acme", Slug: "acme"},
		companydto.CompanyScanSettingsInput{AdapterType: &bogus})
	var se *apperr.ServiceError
	if !errors.As(err, &se) || se.Code != "BAD_REQUEST" {
		t.Errorf("expected BAD_REQUEST, got %T %v", err, err)
	}
}

// TestAdminRepo_GetAndGetAll_WithSettings covers the settings-populated
// branches of Get, GetAll, and fetchSettingsForCompanies.
func TestAdminRepo_GetAndGetAll_WithSettings(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "Alpha")
	seedScanSettings(t, db, "a", companycommon.AdapterGreenhouse, true)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	view, err := repo.Get(context.Background(), "a")
	if err != nil || view.ScanSettings == nil {
		t.Fatalf("Get with settings: view=%+v err=%v", view, err)
	}
	all, err := repo.GetAll(context.Background())
	if err != nil || len(all) != 1 || all[0].ScanSettings == nil {
		t.Fatalf("GetAll with settings: %+v err=%v", all, err)
	}
}

// TestAdminRepo_BadCursorTreatedAsFirstPage covers decodeAdminCursor's
// invalid-base64 branch — a malformed cursor degrades to the first page.
func TestAdminRepo_BadCursorTreatedAsFirstPage(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "Alpha")
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	res, err := repo.List(context.Background(), companydto.CompanyFilter{Cursor: "!!!not-base64!!!"}, 10)
	if err != nil {
		t.Fatalf("List with bad cursor: %v", err)
	}
	if len(res.Items) != 1 {
		t.Errorf("bad cursor should return first page, got %d items", len(res.Items))
	}
}

// TestPublicRepo_BadCursorTreatedAsFirstPage covers decodePublicCursor's
// invalid-base64 branch.
func TestPublicRepo_BadCursorTreatedAsFirstPage(t *testing.T) {
	db := newPublicTestDB(t)
	if err := db.Create(&companymodel.Company{ID: "a", Name: "Alpha", Slug: "a"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := companysvc.NewRepositoryService(db)
	res, err := repo.ListPublic(context.Background(), companydto.CompanyFilter{Cursor: "!!!bad!!!"}, 10)
	if err != nil {
		t.Fatalf("ListPublic bad cursor: %v", err)
	}
	if len(res.Items) != 1 {
		t.Errorf("bad cursor should return first page, got %d items", len(res.Items))
	}
}

// TestAdminRepo_UpdatePublic_AllFields covers publicInputToFields' careers_url
// and logo_url branches.
func TestAdminRepo_UpdatePublic_AllFields(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "Alpha")
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())
	careers := "https://a.com/jobs"
	logo := "https://a.com/logo.png"
	slug := "alpha-co"
	if err := repo.UpdatePublic(context.Background(), "a", companydto.CompanyPublicInput{
		Slug: slug, CareersURL: &careers, LogoURL: &logo,
	}); err != nil {
		t.Fatalf("UpdatePublic: %v", err)
	}
	var got companymodel.Company
	_ = db.First(&got, "id = ?", "a").Error
	if got.Slug != slug || got.CareersURL == nil || *got.CareersURL != careers || got.LogoURL == nil || *got.LogoURL != logo {
		t.Errorf("fields not all plumbed: %+v", got)
	}
}

// TestPublicRepo_ListPublic_SortColumnsAndCursor exercises the public repo
// created_at / updated_at sort columns and the cursor round-trip — covering
// publicSortValue's non-name branches and decodePublicCursor on a real cursor.
func TestPublicRepo_ListPublic_SortColumnsAndCursor(t *testing.T) {
	db := newPublicTestDB(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, nm := range []string{"Alpha", "Bravo", "Charlie"} {
		id := nm
		ct := base.AddDate(0, 0, i)
		if err := db.Create(&companymodel.Company{ID: id, Name: nm, Slug: id, CreatedAt: ct, UpdatedAt: ct}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	repo := companysvc.NewRepositoryService(db)

	for _, sortBy := range []string{"created_at", "updated_at"} {
		res, err := repo.ListPublic(context.Background(), companydto.CompanyFilter{SortBy: sortBy, SortDir: "asc"}, 2)
		if err != nil {
			t.Fatalf("ListPublic %s: %v", sortBy, err)
		}
		if res.NextCursor == nil {
			t.Fatalf("%s: expected a next cursor with 3 rows / limit 2", sortBy)
		}
		// Feed the cursor back: page 2 must return the remaining row.
		res2, err := repo.ListPublic(context.Background(),
			companydto.CompanyFilter{SortBy: sortBy, SortDir: "asc", Cursor: *res.NextCursor}, 2)
		if err != nil {
			t.Fatalf("ListPublic %s page2: %v", sortBy, err)
		}
		if len(res2.Items) != 1 {
			t.Errorf("%s page2 = %d items, want 1", sortBy, len(res2.Items))
		}
	}

	// DESC name cursor branch.
	res, err := repo.ListPublic(context.Background(), companydto.CompanyFilter{SortDir: "desc"}, 2)
	if err != nil {
		t.Fatalf("ListPublic desc: %v", err)
	}
	if res.NextCursor != nil {
		res2, err := repo.ListPublic(context.Background(), companydto.CompanyFilter{SortDir: "desc", Cursor: *res.NextCursor}, 2)
		if err != nil {
			t.Fatalf("ListPublic desc page2: %v", err)
		}
		if len(res2.Items) == 0 {
			t.Errorf("desc page2 should return remaining rows")
		}
	}
}

// TestAdminRepo_NilLogConstructed verifies NewAdminRepositoryService accepts a
// nil log (defaults to noop) — covers the nil-log branch.
func TestAdminRepo_NilLogConstructed(t *testing.T) {
	db, hist := newAdminTestDB(t)
	seedAdminCompany(t, db, "a", "Alpha")
	repo := companysvc.NewAdminRepositoryService(db, hist, nil)
	if _, err := repo.Get(context.Background(), "a"); err != nil {
		t.Fatalf("Get with nil log: %v", err)
	}
}

// TestAdminRepo_List_LastScanAtSort_DescNullBucket exercises the DESC NULL
// bucket cursor branch directly: page into the NULL bucket and back.
func TestAdminRepo_List_LastScanAtSort_DescNullBucket(t *testing.T) {
	db, hist := newAdminTestDB(t)
	now := time.Now()
	mk := func(id string, scan *time.Time) {
		seedAdminCompany(t, db, id, id)
		if err := db.Create(&companymodel.CompanyScanSettings{CompanyID: id, AdapterType: companycommon.AdapterGreenhouse, Active: true, LastScanAt: scan}).Error; err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	mk("a", &now)
	mk("b", nil)
	mk("c", nil)
	repo := companysvc.NewAdminRepositoryService(db, hist, logger.NewNoop())

	// limit 1 forces stepping through each bucket boundary.
	got := paginateAll(t, repo, "last_scan_at", "desc", 1)
	assertNoGapNoOverlap(t, got, []string{"a", "b", "c"}, "last_scan_at/desc limit1")
}
