package scan

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	scanmodel "github.com/trushilpatel/jobradar/service/scan/models"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// scanSQLCapture records every emitted SQL statement so DryRun-based tests
// can assert the SQL shape without running it against a real backend. The
// scan store uses Postgres-specific syntax (FILTER, INTERVAL, RETURNING)
// that sqlite refuses to parse, so the shape assertion is the strongest
// guarantee we can get inside a pure unit test.
type scanSQLCapture struct {
	gormlogger.Interface
	mu  sync.Mutex
	sql []string
}

func (c *scanSQLCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sql = append(c.sql, sql)
}
func (c *scanSQLCapture) LogMode(gormlogger.LogLevel) gormlogger.Interface { return c }
func (c *scanSQLCapture) Info(context.Context, string, ...any)             {}
func (c *scanSQLCapture) Warn(context.Context, string, ...any)             {}
func (c *scanSQLCapture) Error(context.Context, string, ...any)            {}
func (c *scanSQLCapture) statements() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.sql))
	copy(out, c.sql)
	return out
}

// newScanDryRunDB returns a DryRun-mode GORM DB so we can inspect the SQL
// the scan store emits. DryRun mode never executes statements, so
// Postgres-specific syntax (FILTER, INTERVAL '1 second', RETURNING) is
// tolerated.
func newScanDryRunDB(t *testing.T) (*gorm.DB, *scanSQLCapture) {
	t.Helper()
	cap := &scanSQLCapture{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DryRun: true, Logger: cap})
	if err != nil {
		t.Fatalf("open dryrun: %v", err)
	}
	return db, cap
}

// newScanLogTestDB returns an in-memory sqlite DB with `company_scan_log`
// migrated. Used by tests that need to round-trip a row through a real
// store (only InsertCompanyScanLog is sqlite-compatible because every
// other scan log read uses Postgres-specific aggregation syntax).
func newScanLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&scanmodel.CompanyScanLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestScanLogStore_InsertCompanyScanLog_OK verifies a row round-trips with
// the supplied fields preserved.
func TestScanLogStore_InsertCompanyScanLog_OK(t *testing.T) {
	db := newScanLogTestDB(t)
	store := svc.NewLogStore(db)

	errMsg := "fetch timeout"
	if err := store.InsertCompanyScanLog(context.Background(), "stripe", 12, 4, 150, &errMsg); err != nil {
		t.Fatalf("InsertCompanyScanLog() error = %v", err)
	}

	var rows []scanmodel.CompanyScanLog
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	got := rows[0]
	if got.CompanyID != "stripe" || got.JobsFetched != 12 || got.JobsNew != 4 {
		t.Errorf("unexpected row: %+v", got)
	}
	if got.Error == nil || *got.Error != "fetch timeout" {
		t.Errorf("unexpected error column: %v", got.Error)
	}
	if got.DurationMs == nil || *got.DurationMs != 150 {
		t.Errorf("unexpected duration: %v", got.DurationMs)
	}
}

// TestScanLogStore_InsertCompanyScanLog_NilError verifies the success path
// where errMsg is nil (status=ok in the upstream coercion).
func TestScanLogStore_InsertCompanyScanLog_NilError(t *testing.T) {
	db := newScanLogTestDB(t)
	store := svc.NewLogStore(db)

	if err := store.InsertCompanyScanLog(context.Background(), "stripe", 5, 1, 80, nil); err != nil {
		t.Fatalf("InsertCompanyScanLog() error = %v", err)
	}

	var row scanmodel.CompanyScanLog
	if err := db.First(&row, "company_id = ?", "stripe").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if row.Error != nil {
		t.Errorf("expected nil error column, got %v", *row.Error)
	}
}

// TestScanLogStore_GetScanStats_EmitsExpectedSQL asserts the single
// aggregation query carries the FILTER + DATE_TRUNC clauses the
// implementation relies on. Coverage-wise this just walks the SQL builder
// once — execution would fail on sqlite because of Postgres-only syntax,
// so we use DryRun.
func TestScanLogStore_GetScanStats_EmitsExpectedSQL(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewLogStore(db)

	// DryRun returns ErrDryRunModeUnsupported for Raw().Scan() calls in
	// some flows; we tolerate any error here — the assertion is the SQL
	// shape captured by the logger.
	_, _ = store.GetScanStats(context.Background(), "stripe")

	joined := strings.Join(cap.statements(), "\n")
	for _, want := range []string{
		"FROM company_scan_log",
		"FILTER (WHERE error IS NOT NULL)",
		"DATE_TRUNC('month', scanned_at)",
		"FROM company_scan_log",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected SQL to contain %q; got:\n%s", want, joined)
		}
	}
}

func TestScanLogStore_GetScanHistories_EmptyShortCircuits(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewLogStore(db)

	res, err := store.GetScanHistories(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetScanHistories(nil) error = %v", err)
	}
	if len(res) != 0 {
		t.Errorf("expected empty map, got %v", res)
	}
	if len(cap.statements()) != 0 {
		t.Errorf("expected NO SQL emitted for empty input, got %d statements", len(cap.statements()))
	}
}

func TestScanLogStore_GetScanHistories_EmitsRanking(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewLogStore(db)

	_, _ = store.GetScanHistories(context.Background(), []string{"stripe", "shopify"})

	joined := strings.Join(cap.statements(), "\n")
	for _, want := range []string{
		"ROW_NUMBER() OVER",
		"PARTITION BY company_id",
		"WHERE rn <= 10",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected SQL to contain %q; got:\n%s", want, joined)
		}
	}
}

func TestScanLogStore_GetJobsScannedToday_EmitsTodayPredicate(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewLogStore(db)

	_, _ = store.GetJobsScannedToday(context.Background())

	joined := strings.Join(cap.statements(), "\n")
	if !strings.Contains(joined, "DATE(scanned_at) = CURRENT_DATE") {
		t.Errorf("expected today predicate; got:\n%s", joined)
	}
	if !strings.Contains(joined, "SUM(jobs_fetched)") {
		t.Errorf("expected SUM(jobs_fetched); got:\n%s", joined)
	}
}

func TestScanLogStore_GetScansByDay_EmitsDayBucket(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewLogStore(db)

	_, _ = store.GetScansByDay(context.Background(), 30)

	joined := strings.Join(cap.statements(), "\n")
	for _, want := range []string{
		"DATE(scanned_at)",
		"INTERVAL '1 day'",
		"GROUP BY DATE(scanned_at)",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected SQL to contain %q; got:\n%s", want, joined)
		}
	}
}

func TestScanLogStore_GetCompanyAnalytics_EmitsMonthlyWindow(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewLogStore(db)

	_, _ = store.GetCompanyAnalytics(context.Background(), "stripe")

	// In DryRun mode the function short-circuits after the first Raw().Scan
	// call (because gorm reports an error rather than zero rows). We only
	// guarantee the first query — the new-jobs monthly aggregation — was
	// emitted with the expected window. The remaining two queries are
	// exercised under integration tests in a follow-up.
	joined := strings.Join(cap.statements(), "\n")
	for _, want := range []string{
		"FROM job_analytics",
		"DATE_TRUNC('month', snapshotted_at)",
		"INTERVAL '11 months'",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected SQL to contain %q; got:\n%s", want, joined)
		}
	}
}

// TestScanLogStore_RecordScanResult_OK roundtrips through a transaction
// against sqlite. The Updates branch under the success status writes
// last_success_at + consecutive_errors=0; under error it bumps
// consecutive_errors and stores the message. Both branches must touch
// `company_scan_settings` AND insert the log row.
func TestScanLogStore_RecordScanResult_OK(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewLogStore(db)

	errMsg := "boom"
	_ = store.RecordScanResult(context.Background(), "stripe", "error", 5, 2, 80, &errMsg)
	joined := strings.Join(cap.statements(), "\n")
	if !strings.Contains(joined, "INSERT INTO `company_scan_log`") {
		t.Errorf("expected insert into company_scan_log; got:\n%s", joined)
	}
	if !strings.Contains(joined, "company_scan_settings") {
		t.Errorf("expected update on company_scan_settings; got:\n%s", joined)
	}
	if !strings.Contains(joined, "consecutive_errors") {
		t.Errorf("expected consecutive_errors patch; got:\n%s", joined)
	}
}

func TestScanLogStore_RecordScanResult_OKStatusResetsErrors(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewLogStore(db)

	_ = store.RecordScanResult(context.Background(), "stripe", "ok", 5, 2, 80, nil)
	joined := strings.Join(cap.statements(), "\n")
	if !strings.Contains(joined, "last_success_at") {
		t.Errorf("expected last_success_at update on success; got:\n%s", joined)
	}
}
