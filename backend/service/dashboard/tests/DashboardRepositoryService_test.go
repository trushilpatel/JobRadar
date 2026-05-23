package dashboard_test

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/trushilpatel/jobradar/service/dashboard/services"
)

// freshDB returns a usable :memory: sqlite GORM instance. The dashboard
// repository runs Postgres-specific SQL (DATE_TRUNC, FILTER, materialized
// views) that sqlite cannot parse — every query returns an error. We use
// this to exercise the error-return branch of every repository method, which
// is the only branch reachable without a real Postgres backend.
func freshDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

// closedDashboardDB returns a *gorm.DB whose underlying *sql.DB is closed.
// Every subsequent op surfaces "sql: database is closed", deterministically
// exercising error-mapping branches in the repository methods.
func closedDashboardDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := freshDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

// ---------- UserDashboardStore — error paths ----------

// Every UserDashboardStore method must return a non-nil error when the
// underlying DB is unavailable. The error surfaces as a *gorm error which the
// impl service wraps into ErrInternal — that mapping is covered in
// DashboardUserImplService_test.go; here we only confirm the propagation path.

func TestUserDashboardStore_GetProgressStats_DBError(t *testing.T) {
	store := services.NewUserDashboardStore(closedDashboardDB(t))
	if _, err := store.GetProgressStats(context.Background(), "user-1"); err == nil {
		t.Fatal("expected DB error, got nil")
	}
}

func TestUserDashboardStore_GetMarketStats_DBError(t *testing.T) {
	store := services.NewUserDashboardStore(closedDashboardDB(t))
	if _, err := store.GetMarketStats(context.Background()); err == nil {
		t.Fatal("expected DB error, got nil")
	}
}

func TestUserDashboardStore_GetWeeklyHiringActivity_DBError(t *testing.T) {
	store := services.NewUserDashboardStore(closedDashboardDB(t))
	if _, err := store.GetWeeklyHiringActivity(context.Background(), 52); err == nil {
		t.Fatal("expected DB error, got nil")
	}
}

func TestUserDashboardStore_GetCompanyVelocityDetailed_DBError(t *testing.T) {
	store := services.NewUserDashboardStore(closedDashboardDB(t))
	if _, err := store.GetCompanyVelocityDetailed(context.Background(), 10); err == nil {
		t.Fatal("expected DB error, got nil")
	}
}

func TestUserDashboardStore_GetTechStackQuarterlyAdoption_DBError(t *testing.T) {
	store := services.NewUserDashboardStore(closedDashboardDB(t))
	if _, err := store.GetTechStackQuarterlyAdoption(context.Background()); err == nil {
		t.Fatal("expected DB error, got nil")
	}
}

// ---------- PipelineStore — error paths ----------

func TestPipelineStore_GetPipelineDBStats_DBError(t *testing.T) {
	store := services.NewPipelineStore(closedDashboardDB(t))
	if _, err := store.GetPipelineDBStats(context.Background()); err == nil {
		t.Fatal("expected DB error, got nil")
	}
}

// PipelineStore reads from `mv_pipeline_db_stats`; the view doesn't exist in
// sqlite so a fresh DB also produces an error. We cover both branches.
func TestPipelineStore_GetPipelineDBStats_FreshSqliteAlsoErrors(t *testing.T) {
	store := services.NewPipelineStore(freshDB(t))
	if _, err := store.GetPipelineDBStats(context.Background()); err == nil {
		t.Fatal("expected error querying non-existent materialized view")
	}
}

// ---------- StatsRefresher ----------

// REFRESH MATERIALIZED VIEW is Postgres-only — both methods must surface the
// sqlite syntax error rather than silently succeed. This pins the contract
// that the refresher is opaque to the DB driver: any underlying error is
// returned verbatim.
func TestDashboardRefresher_RefreshPipelineStats_Error(t *testing.T) {
	r := services.NewDashboardRefresher(freshDB(t))
	if err := r.RefreshPipelineStats(context.Background()); err == nil {
		t.Fatal("expected error (Postgres-only SQL on sqlite), got nil")
	}
}

func TestDashboardRefresher_RefreshHeatmap_Error(t *testing.T) {
	r := services.NewDashboardRefresher(freshDB(t))
	if err := r.RefreshHeatmap(context.Background()); err == nil {
		t.Fatal("expected error (Postgres-only SQL on sqlite), got nil")
	}
}

// Confirms the refresher constructor returns a non-nil StatsRefresher when
// given a real *gorm.DB.
func TestDashboardRefresher_ConstructorReturnsNonNil(t *testing.T) {
	r := services.NewDashboardRefresher(freshDB(t))
	if r == nil {
		t.Fatal("NewDashboardRefresher returned nil")
	}
}
