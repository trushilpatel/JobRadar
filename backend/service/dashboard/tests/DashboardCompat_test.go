package dashboard_test

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/trushilpatel/jobradar/service/dashboard"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
	"github.com/trushilpatel/jobradar/service/dashboard/services"
	insightsdto "github.com/trushilpatel/jobradar/service/insights/dto"
)

// newSqliteDB returns an in-memory sqlite DB suitable for exercising
// constructor functions that only need a *gorm.DB reference. The schemas
// themselves are not Postgres-compatible at the SQL level, so we do not run
// any queries here — we only need the *gorm.DB handle to be non-nil.
func newSqliteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

// compat.NewService is a thin alias around services.NewDashboardImplService.
// The compat layer exists so callers outside the services package can keep
// their import paths stable across refactors.
func TestCompat_NewService(t *testing.T) {
	svc := dashboard.NewService(
		&fakeScanLogReader{},
		&fakeInsightsService{pipeline: insightsdto.PipelineCounts{}},
		&fakeJobCountReader{},
	)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	// Smoke call to confirm the wiring works end-to-end.
	if _, err := svc.GetStats(context.Background()); err != nil {
		t.Errorf("GetStats: %v", err)
	}
}

// compat.NewUserDashboardStore must return a non-nil store backed by *gorm.DB.
func TestCompat_NewUserDashboardStore(t *testing.T) {
	store := dashboard.NewUserDashboardStore(newSqliteDB(t))
	if store == nil {
		t.Fatal("NewUserDashboardStore returned nil")
	}
}

// compat.NewUserDashboardService wires a store + app reader. We can't make
// progress calls here (Postgres-only SQL), so we assert the constructor
// produces a non-nil service.
func TestCompat_NewUserDashboardService(t *testing.T) {
	svc := dashboard.NewUserDashboardService(
		&fakeUserDashboardStore{},
		&fakeApplicationListReader{},
	)
	if svc == nil {
		t.Fatal("NewUserDashboardService returned nil")
	}
	// Smoke: read a clean progress stats through the wired service.
	if _, err := svc.GetProgressStats(context.Background()); err != nil {
		t.Errorf("GetProgressStats: %v", err)
	}
}

// compat.NewPipelineStore must return a non-nil PipelineStore.
func TestCompat_NewPipelineStore(t *testing.T) {
	store := dashboard.NewPipelineStore(newSqliteDB(t))
	if store == nil {
		t.Fatal("NewPipelineStore returned nil")
	}
}

// compat.NewPipelineService must return a working PipelineService — exercises
// the assembled chain via a known-good drainer call.
func TestCompat_NewPipelineService(t *testing.T) {
	svc := dashboard.NewPipelineService(
		&fakePipelineStore{stats: dto.PipelineDBStats{TotalCompanies: 1}},
		&fakeLagReader{lags: map[string]int64{}},
		&fakeDrainer{drained: 9},
	)
	if svc == nil {
		t.Fatal("NewPipelineService returned nil")
	}
	got, err := svc.TriggerRetry(context.Background(), "filter", "1m")
	if err != nil {
		t.Fatalf("TriggerRetry: %v", err)
	}
	if got != 9 {
		t.Errorf("drained = %d, want 9", got)
	}
}

// compat.NewPipelineRestRouter must produce a non-nil router wrapping the
// PipelineRestService.
func TestCompat_NewPipelineRestRouter(t *testing.T) {
	svc := services.NewPipelineService(
		&fakePipelineStore{},
		&fakeLagReader{lags: map[string]int64{}},
		&fakeDrainer{},
	)
	router := dashboard.NewPipelineRestRouter(svc)
	if router == nil {
		t.Fatal("NewPipelineRestRouter returned nil")
	}
}
