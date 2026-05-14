package company_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// newPublicTestDB returns an in-memory sqlite DB with `companies` migrated.
// SQLite supports NULLS LAST (>=3.30) so the public chain queries run as-is,
// except for ILIKE (Postgres-only). Tests that need to exercise the Search
// branch use the DryRun-capture pattern instead.
func newPublicTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&companymodel.Company{}); err != nil {
		t.Fatalf("migrate companies: %v", err)
	}
	return db
}

// publicSQLCapture is a GORM logger that records every emitted SQL statement.
// Re-implemented here (rather than imported from the internal test file) so
// the blackbox tests package stays self-contained.
type publicSQLCapture struct {
	gormlogger.Interface
	mu  sync.Mutex
	sql []string
}

func (c *publicSQLCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sql = append(c.sql, sql)
}

func (c *publicSQLCapture) LogMode(gormlogger.LogLevel) gormlogger.Interface { return c }
func (c *publicSQLCapture) Info(context.Context, string, ...any)             {}
func (c *publicSQLCapture) Warn(context.Context, string, ...any)             {}
func (c *publicSQLCapture) Error(context.Context, string, ...any)            {}
func (c *publicSQLCapture) statements() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.sql))
	copy(out, c.sql)
	return out
}

// newDryRunDB returns a DryRun-mode DB so we can assert SQL shape without
// touching a real backend. Used for ILIKE / NULLS-LAST branches where running
// against sqlite would error.
func newDryRunDB(t *testing.T) (*gorm.DB, *publicSQLCapture) {
	t.Helper()
	cap := &publicSQLCapture{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DryRun: true, Logger: cap})
	if err != nil {
		t.Fatalf("open dryrun: %v", err)
	}
	return db, cap
}

func TestPublicRepo_GetPublic_FoundAndMissing(t *testing.T) {
	db := newPublicTestDB(t)
	if err := db.Create(&companymodel.Company{ID: "stripe", Name: "Stripe", Slug: "stripe"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := companysvc.NewRepositoryService(db)

	view, err := repo.GetPublic(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("GetPublic: %v", err)
	}
	if view == nil || view.ID != "stripe" {
		t.Errorf("unexpected view: %+v", view)
	}

	// NOT_FOUND for missing id — service returns ErrNotFound, not a raw
	// gorm.ErrRecordNotFound, so handlers can map it to 404.
	if _, err := repo.GetPublic(context.Background(), "missing"); err == nil {
		t.Fatal("expected NOT_FOUND for missing id, got nil")
	} else if se, ok := err.(*apperr.ServiceError); !ok || se.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND ServiceError, got %T %v", err, err)
	}
}

func TestPublicRepo_ListPublic_NoFilter(t *testing.T) {
	db := newPublicTestDB(t)
	for _, name := range []string{"Acme", "Beta", "Gamma"} {
		if err := db.Create(&companymodel.Company{ID: name, Name: name, Slug: strings.ToLower(name)}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	repo := companysvc.NewRepositoryService(db)

	res, err := repo.ListPublic(context.Background(), companydto.CompanyFilter{}, 1, 10)
	if err != nil {
		t.Fatalf("ListPublic: %v", err)
	}
	if res.Total != 3 {
		t.Errorf("Total = %d, want 3", res.Total)
	}
	if len(res.Items) != 3 {
		t.Errorf("Items = %d, want 3", len(res.Items))
	}
	// Default sort is name ASC.
	if got, want := res.Items[0].Name, "Acme"; got != want {
		t.Errorf("first item = %s, want %s", got, want)
	}
}

func TestPublicRepo_ListPublic_Pagination(t *testing.T) {
	db := newPublicTestDB(t)
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		_ = db.Create(&companymodel.Company{ID: name, Name: name, Slug: name}).Error
	}
	repo := companysvc.NewRepositoryService(db)

	// Page 2 limit 2 → items c, d.
	res, err := repo.ListPublic(context.Background(), companydto.CompanyFilter{}, 2, 2)
	if err != nil {
		t.Fatalf("ListPublic: %v", err)
	}
	if res.Total != 5 {
		t.Errorf("Total = %d, want 5", res.Total)
	}
	if len(res.Items) != 2 {
		t.Fatalf("Items = %d, want 2", len(res.Items))
	}
	if res.Items[0].Name != "c" || res.Items[1].Name != "d" {
		t.Errorf("page contents = [%s, %s], want [c, d]", res.Items[0].Name, res.Items[1].Name)
	}
}

func TestPublicRepo_ListPublic_SortBranches(t *testing.T) {
	db := newPublicTestDB(t)
	// Seed three rows with deterministic timestamps so we can verify
	// each whitelist branch routes through the right column.
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []companymodel.Company{
		{ID: "a", Name: "Bravo", Slug: "bravo", CreatedAt: base, UpdatedAt: base.Add(2 * time.Hour)},
		{ID: "b", Name: "Alpha", Slug: "alpha", CreatedAt: base.Add(1 * time.Hour), UpdatedAt: base.Add(1 * time.Hour)},
	}
	for _, r := range rows {
		if err := db.Create(&r).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	repo := companysvc.NewRepositoryService(db)

	cases := []struct {
		name    string
		sortBy  string
		sortDir string
		wantID  string // first item
	}{
		{name: "default name asc", sortBy: "", sortDir: "", wantID: "b"},
		{name: "name desc", sortBy: "", sortDir: "desc", wantID: "a"},
		{name: "created_at asc", sortBy: "created_at", sortDir: "asc", wantID: "a"},
		{name: "created_at desc", sortBy: "created_at", sortDir: "desc", wantID: "b"},
		{name: "updated_at asc", sortBy: "updated_at", sortDir: "asc", wantID: "b"},
		{name: "updated_at desc", sortBy: "updated_at", sortDir: "desc", wantID: "a"},
		{name: "unknown sort_by falls through to name", sortBy: "danger", sortDir: "", wantID: "b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := repo.ListPublic(context.Background(),
				companydto.CompanyFilter{SortBy: tc.sortBy, SortDir: tc.sortDir}, 1, 10)
			if err != nil {
				t.Fatalf("ListPublic: %v", err)
			}
			if len(res.Items) == 0 || res.Items[0].ID != tc.wantID {
				got := ""
				if len(res.Items) > 0 {
					got = res.Items[0].ID
				}
				t.Errorf("first item id = %q, want %q", got, tc.wantID)
			}
		})
	}
}

// TestPublicRepo_ListPublic_SearchEmitsILIKE confirms that the Search branch
// emits an ILIKE WHERE clause. SQLite would reject ILIKE at execution, so we
// use DryRun mode and inspect the captured SQL — that exercises the Go path
// without depending on a real Postgres.
func TestPublicRepo_ListPublic_SearchEmitsILIKE(t *testing.T) {
	db, cap := newDryRunDB(t)
	repo := companysvc.NewRepositoryService(db)
	_, _ = repo.ListPublic(context.Background(), companydto.CompanyFilter{Search: "alpha"}, 1, 10)

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
		t.Errorf("expected ILIKE in emitted SQL, captured: %v", captured)
	}
}

// TestPublicRepo_ListPublic_CountError forces the Count step to fail by
// querying after the underlying *sql.DB has been closed. The repo must map
// the gorm error to ErrInternal, never leak it raw.
func TestPublicRepo_ListPublic_CountError(t *testing.T) {
	db := newPublicTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	repo := companysvc.NewRepositoryService(db)

	_, err := repo.ListPublic(context.Background(), companydto.CompanyFilter{}, 1, 10)
	if err == nil {
		t.Fatal("expected error from closed DB, got nil")
	}
	se, ok := err.(*apperr.ServiceError)
	if !ok || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL ServiceError, got %T %v", err, err)
	}
}

// TestPublicRepo_GetPublic_DBError forces the underlying connection to
// fail so the non-NotFound branch surfaces ErrInternal.
func TestPublicRepo_GetPublic_DBError(t *testing.T) {
	db := newPublicTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	repo := companysvc.NewRepositoryService(db)

	_, err := repo.GetPublic(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error from closed DB, got nil")
	}
	se, ok := err.(*apperr.ServiceError)
	if !ok || se.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL ServiceError, got %T %v", err, err)
	}
}
