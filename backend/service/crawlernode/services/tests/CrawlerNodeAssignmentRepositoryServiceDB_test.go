package tests

import (
	"context"
	"database/sql"
	stderrors "errors"
	"strings"
	"sync"
	"testing"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/crawlernode/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// --- DryRun SQL-probe helpers ---

// assignSQLCapture captures GORM-emitted SQL statements without executing them.
type assignSQLCapture struct {
	gormlogger.Interface
	mu  sync.Mutex
	sql []string
}

func (c *assignSQLCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sql = append(c.sql, sql)
}
func (c *assignSQLCapture) LogMode(gormlogger.LogLevel) gormlogger.Interface { return c }
func (c *assignSQLCapture) Info(context.Context, string, ...any)             {}
func (c *assignSQLCapture) Warn(context.Context, string, ...any)             {}
func (c *assignSQLCapture) Error(context.Context, string, ...any)            {}
func (c *assignSQLCapture) statements() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.sql))
	copy(out, c.sql)
	return out
}

// newAssignDryRunDB returns a DryRun GORM DB that never executes SQL.
// Used for Postgres-specific statements (FOR UPDATE SKIP LOCKED, RETURNING, INTERVAL).
func newAssignDryRunDB(t *testing.T) (*gorm.DB, *assignSQLCapture) {
	t.Helper()
	cap := &assignSQLCapture{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DryRun: true, Logger: cap})
	if err != nil {
		t.Fatalf("open dryrun: %v", err)
	}
	return db, cap
}

// --- SQLite exec helpers ---

// assignTestDriverName names a sqlite3 driver registered once per test binary
// that exposes NOW() as a scalar function. heartbeat_at = NOW() is used in
// ExtendHeartbeats.
const assignTestDriverName = "sqlite3_assignqueue_test"

var registerAssignDriver sync.Once

func ensureAssignDriver() {
	registerAssignDriver.Do(func() {
		sql.Register(assignTestDriverName, &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				return conn.RegisterFunc("now", func() string {
					return time.Now().UTC().Format("2006-01-02 15:04:05")
				}, true)
			},
		})
	})
}

// newAssignExecDB returns a real in-memory SQLite DB with a minimal
// company_scan_queue table and NOW() registered for heartbeat updates.
func newAssignExecDB(t *testing.T) *gorm.DB {
	t.Helper()
	ensureAssignDriver()
	db, err := gorm.Open(sqlite.New(sqlite.Config{DriverName: assignTestDriverName, DSN: ":memory:"}), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE company_scan_queue (
			id TEXT PRIMARY KEY,
			company_id TEXT NOT NULL,
			adapter_type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			source TEXT NOT NULL,
			priority INTEGER DEFAULT 0,
			queued_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			started_at TIMESTAMP,
			heartbeat_at TIMESTAMP,
			next_attempt_at TIMESTAMP,
			locked_by TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create company_scan_queue: %v", err)
	}
	return db
}

func seedQueueItem(t *testing.T, db *gorm.DB, id, status, lockedBy string) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO company_scan_queue (id, company_id, adapter_type, status, source, locked_by)
		 VALUES (?, 'co-1', 'greenhouse', ?, 'scheduler', ?)`,
		id, status, lockedBy,
	).Error; err != nil {
		t.Fatalf("seed queue item %s: %v", id, err)
	}
}

// --- DryRun SQL-probe tests ---

// ClaimNextForNode must emit FOR UPDATE SKIP LOCKED and RETURNING.
func TestAssignmentRepositoryService_ClaimNextForNode_SQLShape(t *testing.T) {
	db, cap := newAssignDryRunDB(t)
	store := services.NewAssignmentRepositoryService(db)

	_, _ = store.ClaimNextForNode(context.Background(), "node-1", "greenhouse")

	stmts := cap.statements()
	if len(stmts) == 0 {
		t.Fatal("ClaimNextForNode: no SQL emitted")
	}
	sql := strings.ToUpper(stmts[0])
	if !strings.Contains(sql, "FOR UPDATE SKIP LOCKED") {
		t.Errorf("ClaimNextForNode SQL missing FOR UPDATE SKIP LOCKED:\n%s", stmts[0])
	}
	if !strings.Contains(sql, "RETURNING") {
		t.Errorf("ClaimNextForNode SQL missing RETURNING:\n%s", stmts[0])
	}
}

// ExtendHeartbeats must emit RETURNING id.
func TestAssignmentRepositoryService_ExtendHeartbeats_SQLShape(t *testing.T) {
	db, cap := newAssignDryRunDB(t)
	store := services.NewAssignmentRepositoryService(db)

	_, _, _ = store.ExtendHeartbeats(context.Background(), "node-1", []string{"q-1", "q-2"})

	stmts := cap.statements()
	if len(stmts) == 0 {
		t.Fatal("ExtendHeartbeats: no SQL emitted")
	}
	sql := strings.ToUpper(stmts[0])
	if !strings.Contains(sql, "RETURNING") {
		t.Errorf("ExtendHeartbeats SQL missing RETURNING:\n%s", stmts[0])
	}
}

// ExtendHeartbeats with empty slice must return empty slices without a DB call.
func TestAssignmentRepositoryService_ExtendHeartbeats_EmptySliceNoDBCall(t *testing.T) {
	db, cap := newAssignDryRunDB(t)
	store := services.NewAssignmentRepositoryService(db)

	acked, missed, err := store.ExtendHeartbeats(context.Background(), "node-1", []string{})
	if err != nil {
		t.Fatalf("ExtendHeartbeats(empty) error = %v", err)
	}
	if len(acked) != 0 || len(missed) != 0 {
		t.Errorf("ExtendHeartbeats(empty) acked=%d missed=%d, want 0/0", len(acked), len(missed))
	}
	if len(cap.statements()) != 0 {
		t.Error("ExtendHeartbeats(empty) issued a DB call — must short-circuit")
	}
}

// ResetStuckForNodes must emit INTERVAL arithmetic in the WHERE clause.
func TestAssignmentRepositoryService_ResetStuckForNodes_SQLShape(t *testing.T) {
	db, cap := newAssignDryRunDB(t)
	store := services.NewAssignmentRepositoryService(db)

	n, err := store.ResetStuckForNodes(context.Background(), 5*time.Minute)
	// DryRun never executes — count will be 0, error should be nil.
	if err != nil {
		t.Fatalf("ResetStuckForNodes DryRun error = %v", err)
	}
	if n != 0 {
		t.Errorf("ResetStuckForNodes DryRun count = %d, want 0", n)
	}

	stmts := cap.statements()
	if len(stmts) == 0 {
		t.Fatal("ResetStuckForNodes: no SQL emitted")
	}
	sql := strings.ToUpper(stmts[0])
	if !strings.Contains(sql, "INTERVAL") {
		t.Errorf("ResetStuckForNodes SQL missing INTERVAL:\n%s", stmts[0])
	}
}

// --- SQLite exec tests: ExtendHeartbeats ---

// ExtendHeartbeats on SQLite exercises the acked/missed diff logic.
// SQLite 3.35+ supports RETURNING; go-sqlite3 bundles a compatible version.
func TestAssignmentRepositoryService_ExtendHeartbeats_AckedMissedDiff(t *testing.T) {
	db := newAssignExecDB(t)
	seedQueueItem(t, db, "q-1", "running", "node-1")
	seedQueueItem(t, db, "q-2", "running", "node-other") // locked by different node
	store := services.NewAssignmentRepositoryService(db)

	acked, missed, err := store.ExtendHeartbeats(context.Background(), "node-1", []string{"q-1", "q-2"})
	if err != nil {
		t.Fatalf("ExtendHeartbeats() error = %v", err)
	}
	if len(acked) != 1 || acked[0] != "q-1" {
		t.Errorf("ExtendHeartbeats() acked = %v, want [q-1]", acked)
	}
	if len(missed) != 1 || missed[0] != "q-2" {
		t.Errorf("ExtendHeartbeats() missed = %v, want [q-2]", missed)
	}
}

// --- SQLite exec tests: CompleteForNode ---

func TestAssignmentRepositoryService_CompleteForNode_DeletesRow(t *testing.T) {
	db := newAssignExecDB(t)
	seedQueueItem(t, db, "q-1", "running", "node-1")
	store := services.NewAssignmentRepositoryService(db)

	if err := store.CompleteForNode(context.Background(), "q-1", "node-1"); err != nil {
		t.Fatalf("CompleteForNode() error = %v", err)
	}
	var count int64
	db.Raw("SELECT COUNT(*) FROM company_scan_queue WHERE id = 'q-1'").Scan(&count)
	if count != 0 {
		t.Errorf("CompleteForNode: row still exists (count=%d)", count)
	}
}

func TestAssignmentRepositoryService_CompleteForNode_ErrQueueRowMissingOnOwnershipMismatch(t *testing.T) {
	db := newAssignExecDB(t)
	seedQueueItem(t, db, "q-1", "running", "node-other")
	store := services.NewAssignmentRepositoryService(db)

	err := store.CompleteForNode(context.Background(), "q-1", "node-1")
	if !stderrors.Is(err, domainerrs.ErrQueueRowMissing) {
		t.Errorf("CompleteForNode(mismatch) error = %v, want ErrQueueRowMissing", err)
	}
}

func TestAssignmentRepositoryService_CompleteForNode_ErrQueueRowMissingOnMissingRow(t *testing.T) {
	db := newAssignExecDB(t)
	store := services.NewAssignmentRepositoryService(db)

	err := store.CompleteForNode(context.Background(), "nonexistent", "node-1")
	if !stderrors.Is(err, domainerrs.ErrQueueRowMissing) {
		t.Errorf("CompleteForNode(missing) error = %v, want ErrQueueRowMissing", err)
	}
}

// --- FailForNode (back-off) ---

// FailForNode now uses Postgres-only correlated-UPDATE syntax (FROM
// company_scan_settings, INTERVAL, POWER, LEAST) to stamp next_attempt_at, so
// its execution semantics live in the integration tier. Here we probe the SQL
// shape under DryRun to prove the back-off columns and the ownership guard are
// present, and use a real (executing) SQLite DB only to exercise the
// RowsAffected==0 → ErrQueueRowMissing ownership-mismatch branch, which does not
// depend on the Postgres-only functions (0 rows match, nothing is computed).

// FailForNode SQL must set next_attempt_at with the exponential back-off
// (INTERVAL + POWER + LEAST), join company_scan_settings, and keep the
// locked_by ownership guard.
func TestAssignmentRepositoryService_FailForNode_SQLShape_BackoffAndGuard(t *testing.T) {
	db, cap := newAssignDryRunDB(t)
	store := services.NewAssignmentRepositoryService(db)

	_ = store.FailForNode(context.Background(), "q-1", "node-1")

	stmts := cap.statements()
	if len(stmts) == 0 {
		t.Fatal("FailForNode: no SQL emitted")
	}
	upper := strings.ToUpper(stmts[0])
	for _, want := range []string{
		"NEXT_ATTEMPT_AT",
		"FROM COMPANY_SCAN_SETTINGS",
		"POWER(",
		"LEAST(",
		"INTERVAL",
		"CONSECUTIVE_ERRORS",
		"LOCKED_BY =",
	} {
		if !strings.Contains(upper, want) {
			t.Errorf("FailForNode SQL missing %q:\n%s", want, stmts[0])
		}
	}
}

// FailForNode's RowsAffected==0 → ErrQueueRowMissing ownership-mismatch branch
// is exercised in the integration tier: the correlated UPDATE ... FROM
// company_scan_settings syntax (with POWER/LEAST/INTERVAL) is Postgres-only and
// SQLite cannot parse it, so a real round-trip needs Postgres. The closed-DB
// error test below and the DryRun shape test above cover the unit surface.

// ClaimNextForNode SQL must gate on next_attempt_at so a backed-off row is not
// re-served instantly (both the adapter-scoped and generic SELECTs).
func TestAssignmentRepositoryService_ClaimNextForNode_DueGate_SQLShape(t *testing.T) {
	for _, adapter := range []string{"greenhouse", ""} {
		db, cap := newAssignDryRunDB(t)
		store := services.NewAssignmentRepositoryService(db)

		_, _ = store.ClaimNextForNode(context.Background(), "node-1", adapter)

		stmts := cap.statements()
		if len(stmts) == 0 {
			t.Fatalf("ClaimNextForNode(%q): no SQL emitted", adapter)
		}
		upper := strings.ToUpper(stmts[0])
		if !strings.Contains(upper, "NEXT_ATTEMPT_AT") {
			t.Errorf("ClaimNextForNode(%q) SQL missing next_attempt_at due-gate:\n%s", adapter, stmts[0])
		}
	}
}

// --- Closed-DB error paths ---

func newClosedAssignDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newAssignExecDB(t)
	raw, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

func TestAssignmentRepositoryService_ClaimNextForNode_ErrInternalOnDBError(t *testing.T) {
	store := services.NewAssignmentRepositoryService(newClosedAssignDB(t))
	_, err := store.ClaimNextForNode(context.Background(), "node-1", "greenhouse")
	if err == nil {
		t.Fatal("ClaimNextForNode() on closed DB expected error, got nil")
	}
}

func TestAssignmentRepositoryService_ExtendHeartbeats_ErrInternalOnDBError(t *testing.T) {
	store := services.NewAssignmentRepositoryService(newClosedAssignDB(t))
	_, _, err := store.ExtendHeartbeats(context.Background(), "node-1", []string{"q-1"})
	if err == nil {
		t.Fatal("ExtendHeartbeats() on closed DB expected error, got nil")
	}
}

func TestAssignmentRepositoryService_CompleteForNode_ErrInternalOnDBError(t *testing.T) {
	store := services.NewAssignmentRepositoryService(newClosedAssignDB(t))
	err := store.CompleteForNode(context.Background(), "q-1", "node-1")
	if err == nil {
		t.Fatal("CompleteForNode() on closed DB expected error, got nil")
	}
}

func TestAssignmentRepositoryService_FailForNode_ErrInternalOnDBError(t *testing.T) {
	store := services.NewAssignmentRepositoryService(newClosedAssignDB(t))
	err := store.FailForNode(context.Background(), "q-1", "node-1")
	if err == nil {
		t.Fatal("FailForNode() on closed DB expected error, got nil")
	}
}

func TestAssignmentRepositoryService_ResetStuckForNodes_ErrInternalOnDBError(t *testing.T) {
	store := services.NewAssignmentRepositoryService(newClosedAssignDB(t))
	_, err := store.ResetStuckForNodes(context.Background(), 5*time.Minute)
	if err == nil {
		t.Fatal("ResetStuckForNodes() on closed DB expected error, got nil")
	}
}

// ClaimNextForNode with empty adapterType must NOT include "AND adapter_type = ?" in the WHERE clause.
func TestAssignmentRepositoryService_ClaimNextForNode_EmptyAdapterType_SQLShape(t *testing.T) {
	db, cap := newAssignDryRunDB(t)
	store := services.NewAssignmentRepositoryService(db)

	_, _ = store.ClaimNextForNode(context.Background(), "node-1", "")

	stmts := cap.statements()
	if len(stmts) == 0 {
		t.Fatal("ClaimNextForNode (empty adapter): no SQL emitted")
	}
	sql := strings.ToUpper(stmts[0])
	// The RETURNING clause always mentions adapter_type; the WHERE clause must not.
	if strings.Contains(sql, "AND ADAPTER_TYPE") {
		t.Errorf("ClaimNextForNode (empty adapter) SQL must not filter on adapter_type in WHERE:\n%s", stmts[0])
	}
	if !strings.Contains(sql, "FOR UPDATE SKIP LOCKED") {
		t.Errorf("ClaimNextForNode (empty adapter) SQL missing FOR UPDATE SKIP LOCKED:\n%s", stmts[0])
	}
}

// ClaimNextForNode with empty adapterType must propagate DB errors.
func TestAssignmentRepositoryService_ClaimNextForNode_EmptyAdapterType_ErrInternalOnDBError(t *testing.T) {
	store := services.NewAssignmentRepositoryService(newClosedAssignDB(t))
	_, err := store.ClaimNextForNode(context.Background(), "node-1", "")
	if err == nil {
		t.Fatal("ClaimNextForNode (empty adapter, closed DB) expected error, got nil")
	}
}
