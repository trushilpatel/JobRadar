package scan

import (
	"context"
	"database/sql"
	stderrors "errors"
	"strings"
	"sync"
	"testing"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// scanQueueTestDriver names a sqlite3 driver registered once per test binary
// that exposes NOW() as a scalar function. UpdateHeartbeat uses SET
// heartbeat_at = NOW(), which Postgres provides natively but sqlite does not.
const scanQueueTestDriver = "sqlite3_scanqueue_test"

var registerScanQueueDriver sync.Once

func ensureScanQueueDriver() {
	registerScanQueueDriver.Do(func() {
		sql.Register(scanQueueTestDriver, &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				return conn.RegisterFunc("now", func() string {
					return time.Now().UTC().Format("2006-01-02 15:04:05")
				}, true)
			},
		})
	})
}

// newScanQueueExecDB returns a real (executing) in-memory sqlite DB with a
// minimal company_scan_queue table. Complete/Remove/UpdateHeartbeat use plain
// DELETE/UPDATE ... WHERE id = ? statements that sqlite executes faithfully, so
// these can be round-tripped (unlike ResetStuck/ClaimNext, which use
// Postgres-only INTERVAL/RETURNING/FOR UPDATE syntax — covered by DryRun above
// and the integration tier). There is no GORM model for the queue table (it is
// raw-SQL only), so the schema is declared here explicitly.
func newScanQueueExecDB(t *testing.T) *gorm.DB {
	t.Helper()
	ensureScanQueueDriver()
	db, err := gorm.Open(sqlite.New(sqlite.Config{DriverName: scanQueueTestDriver, DSN: ":memory:"}), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE company_scan_queue (
			id TEXT PRIMARY KEY,
			company_id TEXT,
			adapter_type TEXT,
			status TEXT,
			source TEXT,
			priority INTEGER DEFAULT 0,
			queued_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			started_at TIMESTAMP,
			heartbeat_at TIMESTAMP
		)
	`).Error; err != nil {
		t.Fatalf("create company_scan_queue: %v", err)
	}
	return db
}

func seedQueueRow(t *testing.T, db *gorm.DB, id, status string) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO company_scan_queue (id, company_id, adapter_type, status, source) VALUES (?, 'c1', 'greenhouse', ?, 'manual')`,
		id, status,
	).Error; err != nil {
		t.Fatalf("seed queue row %s: %v", id, err)
	}
}

// TestScanQueue_Complete_MissingRow_ReturnsErrQueueRowMissing (#11).
func TestScanQueue_Complete_MissingRow_ReturnsErrQueueRowMissing(t *testing.T) {
	t.Parallel()
	db := newScanQueueExecDB(t)
	store := svc.NewQueueStore(db)

	err := store.Complete(context.Background(), "ghost")
	if !stderrors.Is(err, domainerrs.ErrQueueRowMissing) {
		t.Fatalf("want ErrQueueRowMissing, got %v", err)
	}
}

// TestScanQueue_Complete_PresentRow_ReturnsNil — happy path round-trip.
func TestScanQueue_Complete_PresentRow_ReturnsNil(t *testing.T) {
	t.Parallel()
	db := newScanQueueExecDB(t)
	seedQueueRow(t, db, "q1", "running")
	store := svc.NewQueueStore(db)

	if err := store.Complete(context.Background(), "q1"); err != nil {
		t.Fatalf("Complete present row err = %v", err)
	}
}

// TestScanQueue_UpdateHeartbeat_MissingRow_ReturnsErrQueueRowMissing (#12).
func TestScanQueue_UpdateHeartbeat_MissingRow_ReturnsErrQueueRowMissing(t *testing.T) {
	t.Parallel()
	db := newScanQueueExecDB(t)
	store := svc.NewQueueStore(db)

	err := store.UpdateHeartbeat(context.Background(), "ghost")
	if !stderrors.Is(err, domainerrs.ErrQueueRowMissing) {
		t.Fatalf("want ErrQueueRowMissing, got %v", err)
	}
}

// TestScanQueue_UpdateHeartbeat_PresentRow_ReturnsNil.
func TestScanQueue_UpdateHeartbeat_PresentRow_ReturnsNil(t *testing.T) {
	t.Parallel()
	db := newScanQueueExecDB(t)
	seedQueueRow(t, db, "q1", "running")
	store := svc.NewQueueStore(db)

	if err := store.UpdateHeartbeat(context.Background(), "q1"); err != nil {
		t.Fatalf("UpdateHeartbeat present row err = %v", err)
	}
}

// TestScanQueue_Remove_MissingRow_ReturnsErrQueueRowMissing (#13) — a running
// row is not removable, so Remove reports the row missing (queued-only guard).
func TestScanQueue_Remove_MissingRow_ReturnsErrQueueRowMissing(t *testing.T) {
	t.Parallel()
	db := newScanQueueExecDB(t)
	seedQueueRow(t, db, "q-running", "running")
	store := svc.NewQueueStore(db)

	// Absent id → missing.
	if err := store.Remove(context.Background(), "ghost"); !stderrors.Is(err, domainerrs.ErrQueueRowMissing) {
		t.Fatalf("absent id: want ErrQueueRowMissing, got %v", err)
	}
	// Running row → not queued, so the queued-only DELETE matches 0 rows.
	if err := store.Remove(context.Background(), "q-running"); !stderrors.Is(err, domainerrs.ErrQueueRowMissing) {
		t.Fatalf("running row: want ErrQueueRowMissing, got %v", err)
	}
}

// TestScanQueue_Remove_QueuedRow_ReturnsNil.
func TestScanQueue_Remove_QueuedRow_ReturnsNil(t *testing.T) {
	t.Parallel()
	db := newScanQueueExecDB(t)
	seedQueueRow(t, db, "q1", "queued")
	store := svc.NewQueueStore(db)

	if err := store.Remove(context.Background(), "q1"); err != nil {
		t.Fatalf("Remove queued row err = %v", err)
	}
}

// TestScanQueue_ResetStuck_ReturnsRowsResetCount (#14) — ResetStuck uses
// Postgres-only INTERVAL arithmetic, so the executing count path is verified in
// the integration tier. Here we assert the DryRun signature returns (count, nil)
// without error so the new return shape is exercised.
func TestScanQueue_ResetStuck_ReturnsRowsResetCount(t *testing.T) {
	t.Parallel()
	db, _ := newScanDryRunDB(t)
	store := svc.NewQueueStore(db)

	n, err := store.ResetStuck(context.Background(), 90*time.Second)
	if err != nil {
		t.Fatalf("ResetStuck err = %v", err)
	}
	if n != 0 {
		t.Errorf("DryRun ResetStuck count = %d, want 0 (no execution)", n)
	}
}

// The scanQueueStore methods are all backed by Postgres-specific SQL
// (INTERVAL '1 second', ON CONFLICT DO NOTHING, RETURNING, FOR UPDATE SKIP
// LOCKED). Sqlite cannot execute these, so we use DryRun mode of GORM to
// capture the emitted SQL shape and verify each method binds the right
// table, predicates, and constants. This is the same pattern the company
// test suite uses for its Postgres-only branches.

func TestScanQueueStore_Enqueue_EmitsInsertWithConflict(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewQueueStore(db)

	_ = store.Enqueue(context.Background(), "stripe", "manual")

	joined := strings.Join(cap.statements(), "\n")
	for _, want := range []string{
		"INSERT INTO company_scan_queue",
		"FROM company_scan_settings",
		"ON CONFLICT DO NOTHING",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected SQL to contain %q; got:\n%s", want, joined)
		}
	}
}

func TestScanQueueStore_EnqueueDue_EmitsIntervalAndOnConflict(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewQueueStore(db)

	_, _ = store.EnqueueDue(context.Background(), 1440, "scheduler")

	joined := strings.Join(cap.statements(), "\n")
	for _, want := range []string{
		"INSERT INTO company_scan_queue",
		"FROM company_scan_settings",
		"COALESCE(",
		"INTERVAL '1 minute'",
		"ON CONFLICT DO NOTHING",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected SQL to contain %q; got:\n%s", want, joined)
		}
	}
}

func TestScanQueueStore_EnqueueAll_EmitsActiveOnlyInsert(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewQueueStore(db)

	_, _ = store.EnqueueAll(context.Background(), "manual")

	joined := strings.Join(cap.statements(), "\n")
	for _, want := range []string{
		"INSERT INTO company_scan_queue",
		"s.active = true",
		"status IN ('queued','running')",
		"ON CONFLICT DO NOTHING",
		"RETURNING company_id",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected SQL to contain %q; got:\n%s", want, joined)
		}
	}
}

func TestScanQueueStore_ClaimNext_EmitsForUpdateAndSkipLocked(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewQueueStore(db)

	// DryRun + transaction: gorm still emits the statements through the
	// logger when nested in a transaction begun on a DryRun session.
	_, _ = store.ClaimNext(context.Background(), "greenhouse")

	joined := strings.Join(cap.statements(), "\n")
	// At minimum the first lock-check SELECT must be emitted with the
	// FOR UPDATE clause that serialises concurrent pod claims.
	for _, want := range []string{
		"FROM ats",
		"FOR UPDATE",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected SQL to contain %q; got:\n%s", want, joined)
		}
	}
}

func TestScanQueueStore_PurgeInactive_EmitsInactiveCriteria(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewQueueStore(db)

	_, _ = store.PurgeInactive(context.Background())

	joined := strings.Join(cap.statements(), "\n")
	for _, want := range []string{
		"DELETE FROM company_scan_queue",
		"status = 'queued'",
		"company_scan_settings",
		"s.company_id IS NULL OR s.active = false",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected SQL to contain %q; got:\n%s", want, joined)
		}
	}
}

func TestScanQueueStore_Complete_EmitsDelete(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewQueueStore(db)

	_ = store.Complete(context.Background(), "entry-1")

	joined := strings.Join(cap.statements(), "\n")
	if !strings.Contains(joined, "DELETE FROM company_scan_queue WHERE id =") {
		t.Errorf("expected DELETE statement; got:\n%s", joined)
	}
}

func TestScanQueueStore_Remove_EmitsQueuedOnlyGuard(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewQueueStore(db)

	_ = store.Remove(context.Background(), "entry-1")

	joined := strings.Join(cap.statements(), "\n")
	if !strings.Contains(joined, "DELETE FROM company_scan_queue") {
		t.Errorf("expected DELETE statement; got:\n%s", joined)
	}
	if !strings.Contains(joined, "status = 'queued'") {
		t.Errorf("expected Remove to guard against running rows; got:\n%s", joined)
	}
}

func TestScanQueueStore_ResetStuck_EmitsHeartbeatWindow(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewQueueStore(db)

	if _, err := store.ResetStuck(context.Background(), 90*time.Second); err != nil {
		t.Fatalf("ResetStuck() error = %v", err)
	}

	joined := strings.Join(cap.statements(), "\n")
	for _, want := range []string{
		"UPDATE company_scan_queue",
		"SET status = 'queued'",
		"COALESCE(heartbeat_at, started_at)",
		"INTERVAL '1 second'",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected SQL to contain %q; got:\n%s", want, joined)
		}
	}
}

func TestScanQueueStore_UpdateHeartbeat_EmitsUpdate(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewQueueStore(db)

	_ = store.UpdateHeartbeat(context.Background(), "entry-1")

	joined := strings.Join(cap.statements(), "\n")
	if !strings.Contains(joined, "UPDATE company_scan_queue SET heartbeat_at = NOW()") {
		t.Errorf("expected heartbeat UPDATE; got:\n%s", joined)
	}
}

func TestScanQueueStore_ListQueue_NoFilter(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewQueueStore(db)

	_, _ = store.ListQueue(context.Background(), dto.QueueFilter{})

	joined := strings.Join(cap.statements(), "\n")
	for _, want := range []string{
		"FROM company_scan_queue",
		"ORDER BY priority DESC, queued_at ASC",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected SQL to contain %q; got:\n%s", want, joined)
		}
	}
}

func TestScanQueueStore_ListQueue_BothFilters(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewQueueStore(db)

	_, _ = store.ListQueue(context.Background(), dto.QueueFilter{Status: "queued", Source: "manual"})

	joined := strings.Join(cap.statements(), "\n")
	for _, want := range []string{
		"status = ",
		"source = ",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected SQL to contain %q; got:\n%s", want, joined)
		}
	}
}

// --- Closed-DB error-path tests for ScanQueueStore ---
// These exercise the error branches that can't be hit via DryRun mode
// (which never executes SQL and therefore never returns DB errors).

func newClosedScanQueueDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newScanQueueExecDB(t)
	raw, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

func TestScanQueueStore_Complete_DBError(t *testing.T) {
	store := svc.NewQueueStore(newClosedScanQueueDB(t))
	err := store.Complete(context.Background(), "any")
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
	if stderrors.Is(err, domainerrs.ErrQueueRowMissing) {
		t.Error("unexpected ErrQueueRowMissing — should be a DB error")
	}
}

func TestScanQueueStore_Remove_DBError(t *testing.T) {
	store := svc.NewQueueStore(newClosedScanQueueDB(t))
	err := store.Remove(context.Background(), "any")
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
	if stderrors.Is(err, domainerrs.ErrQueueRowMissing) {
		t.Error("unexpected ErrQueueRowMissing — should be a DB error")
	}
}

func TestScanQueueStore_UpdateHeartbeat_DBError(t *testing.T) {
	store := svc.NewQueueStore(newClosedScanQueueDB(t))
	err := store.UpdateHeartbeat(context.Background(), "any")
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
	if stderrors.Is(err, domainerrs.ErrQueueRowMissing) {
		t.Error("unexpected ErrQueueRowMissing — should be a DB error")
	}
}

func TestScanQueueStore_ResetStuck_DBError(t *testing.T) {
	store := svc.NewQueueStore(newClosedScanQueueDB(t))
	if _, err := store.ResetStuck(context.Background(), 30*time.Second); err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestScanQueueStore_ListQueue_DBError(t *testing.T) {
	store := svc.NewQueueStore(newClosedScanQueueDB(t))
	if _, err := store.ListQueue(context.Background(), dto.QueueFilter{}); err == nil {
		t.Fatal("expected error on closed DB")
	}
}
