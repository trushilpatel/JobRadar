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
	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
	"github.com/trushilpatel/jobradar/service/crawlernode/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// --- fake AtsService (backs AdapterDelayGate in these tests) ---

// fakeAtsService is a minimal atssvc.AtsService double. delays maps adapter
// name -> inter_company_delay_s; an adapter absent from the map mirrors
// AtsService.GetATS returning ErrNotFound.
type fakeAtsService struct {
	delays map[string]int
	err    error
}

func (f *fakeAtsService) ListATS(context.Context) ([]atsdto.ATSDTO, error) { return nil, nil }

func (f *fakeAtsService) GetATS(_ context.Context, name string) (atsdto.ATSDTO, error) {
	if f.err != nil {
		return atsdto.ATSDTO{}, f.err
	}
	delay, ok := f.delays[name]
	if !ok {
		return atsdto.ATSDTO{}, domainerrs.ErrNotFound.WithMessage("unknown adapter " + name)
	}
	return atsdto.ATSDTO{Name: name, InterCompanyDelayS: delay}, nil
}

func (f *fakeAtsService) UpdateATS(_ context.Context, _ string, _ atsdto.UpdateATSInput) (atsdto.ATSDTO, error) {
	return atsdto.ATSDTO{}, nil
}

var _ atssvc.AtsService = (*fakeAtsService)(nil)

// alwaysEligibleGate returns a gate over adapters that never block (either
// delay=0 or unused in a given test) so tests that are not exercising the
// delay-gate itself can ignore it.
func alwaysEligibleGate(adapterTypes ...string) *services.AdapterDelayGate {
	delays := make(map[string]int, len(adapterTypes))
	for _, a := range adapterTypes {
		delays[a] = 0
	}
	return services.NewAdapterDelayGate(&fakeAtsService{delays: delays})
}

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
// Used for Postgres-specific statements (RETURNING, INTERVAL).
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

const createQueueTableSQL = `
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
`

// createSettingsTableSQL mirrors the columns listClaimCandidates joins
// against — a minimal projection of company_scan_settings, not the full
// production schema.
const createSettingsTableSQL = `
	CREATE TABLE company_scan_settings (
		company_id TEXT PRIMARY KEY,
		active BOOLEAN NOT NULL DEFAULT 1,
		auto_paused BOOLEAN NOT NULL DEFAULT 0
	)
`

// newAssignExecDB returns a real in-memory SQLite DB with a minimal
// company_scan_queue table and NOW() registered for heartbeat updates.
func newAssignExecDB(t *testing.T) *gorm.DB {
	t.Helper()
	ensureAssignDriver()
	db, err := gorm.Open(sqlite.New(sqlite.Config{DriverName: assignTestDriverName, DSN: ":memory:"}), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(createQueueTableSQL).Error; err != nil {
		t.Fatalf("create company_scan_queue: %v", err)
	}
	if err := db.Exec(createSettingsTableSQL).Error; err != nil {
		t.Fatalf("create company_scan_settings: %v", err)
	}
	return db
}

// newAssignExecDBShared returns a real SQLite DB using a shared-cache
// in-memory database (multiple connections in this process see the same
// data) — needed by the lost-claim-race test below, which injects a
// side-effect UPDATE from a second *sql.DB handle mid-query.
func newAssignExecDBShared(t *testing.T) *gorm.DB {
	t.Helper()
	ensureAssignDriver()
	dsn := "file::memory:?cache=shared&_busy_timeout=5000"
	db, err := gorm.Open(sqlite.New(sqlite.Config{DriverName: assignTestDriverName, DSN: dsn}), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open shared sqlite: %v", err)
	}
	if err := db.Exec(createQueueTableSQL).Error; err != nil {
		t.Fatalf("create company_scan_queue: %v", err)
	}
	if err := db.Exec(createSettingsTableSQL).Error; err != nil {
		t.Fatalf("create company_scan_settings: %v", err)
	}
	return db
}

func seedQueueItem(t *testing.T, db *gorm.DB, id, status, lockedBy string) {
	t.Helper()
	seedQueueItemFull(t, db, id, status, lockedBy, "greenhouse", 0)
}

// seedQueueItemFull seeds a row with an explicit adapter_type and priority so
// ORDER BY priority DESC, queued_at ASC is deterministic across rows inserted
// within the same wall-clock second. Every row here belongs to company 'co-1'
// — seedSettingsRow is called idempotently so any number of calls across a
// test share one active, non-paused settings row for it.
func seedQueueItemFull(t *testing.T, db *gorm.DB, id, status, lockedBy, adapterType string, priority int) {
	t.Helper()
	seedSettingsRow(t, db, "co-1", true, false)
	if err := db.Exec(
		`INSERT INTO company_scan_queue (id, company_id, adapter_type, status, source, locked_by, priority)
		 VALUES (?, 'co-1', ?, ?, 'scheduler', ?, ?)`,
		id, adapterType, status, lockedBy, priority,
	).Error; err != nil {
		t.Fatalf("seed queue item %s: %v", id, err)
	}
}

// seedSettingsRow idempotently inserts (or leaves untouched, on repeat calls
// for the same companyID) a company_scan_settings row with explicit
// active/auto_paused state — the two flags listClaimCandidates gates on.
func seedSettingsRow(t *testing.T, db *gorm.DB, companyID string, active, autoPaused bool) {
	t.Helper()
	if err := db.Exec(
		`INSERT OR IGNORE INTO company_scan_settings (company_id, active, auto_paused) VALUES (?, ?, ?)`,
		companyID, active, autoPaused,
	).Error; err != nil {
		t.Fatalf("seed settings row for %s: %v", companyID, err)
	}
}

// seedQueueItemForCompany seeds a queued row for an explicit company_id with
// its own settings row — used by tests that need a company distinct from the
// shared 'co-1' fixture (e.g. to exercise the active/auto_paused gate).
func seedQueueItemForCompany(t *testing.T, db *gorm.DB, id, companyID, adapterType string, priority int, active, autoPaused bool) {
	t.Helper()
	seedSettingsRow(t, db, companyID, active, autoPaused)
	if err := db.Exec(
		`INSERT INTO company_scan_queue (id, company_id, adapter_type, status, source, priority)
		 VALUES (?, ?, ?, 'queued', 'scheduler', ?)`,
		id, companyID, adapterType, priority,
	).Error; err != nil {
		t.Fatalf("seed queue item %s for company %s: %v", id, companyID, err)
	}
}

// --- DryRun SQL-probe tests ---

// ClaimNextForNode's candidate listing must emit RETURNING-free plain SELECT
// with the priority/queued_at ordering and the due-gate — and must NOT lock
// the batch (no FOR UPDATE): safety comes from the per-row targeted UPDATE,
// not from locking the listing SELECT.
func TestAssignmentRepositoryService_ClaimNextForNode_ListsCandidates_SQLShape(t *testing.T) {
	db, cap := newAssignDryRunDB(t)
	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate("greenhouse"))

	_, _ = store.ClaimNextForNode(context.Background(), "node-1", "greenhouse")

	stmts := cap.statements()
	if len(stmts) == 0 {
		t.Fatal("ClaimNextForNode: no SQL emitted")
	}
	sql := strings.ToUpper(stmts[0])
	if !strings.Contains(sql, "ORDER BY Q.PRIORITY DESC, Q.QUEUED_AT ASC") {
		t.Errorf("ClaimNextForNode SQL missing ORDER BY priority DESC, queued_at ASC:\n%s", stmts[0])
	}
	if strings.Contains(sql, "FOR UPDATE") {
		t.Errorf("ClaimNextForNode candidate listing must not lock the batch (FOR UPDATE):\n%s", stmts[0])
	}
	if !strings.Contains(sql, "STATUS = 'QUEUED'") {
		t.Errorf("ClaimNextForNode SQL missing status='queued' filter:\n%s", stmts[0])
	}
}

// ExtendHeartbeats must emit RETURNING id.
func TestAssignmentRepositoryService_ExtendHeartbeats_SQLShape(t *testing.T) {
	db, cap := newAssignDryRunDB(t)
	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate())

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
	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate())

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
	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate())

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
	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate())

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
	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate())

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
	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate())

	err := store.CompleteForNode(context.Background(), "q-1", "node-1")
	if !stderrors.Is(err, domainerrs.ErrQueueRowMissing) {
		t.Errorf("CompleteForNode(mismatch) error = %v, want ErrQueueRowMissing", err)
	}
}

func TestAssignmentRepositoryService_CompleteForNode_ErrQueueRowMissingOnMissingRow(t *testing.T) {
	db := newAssignExecDB(t)
	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate())

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
	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate())

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
		store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate("greenhouse"))

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
	store := services.NewAssignmentRepositoryService(newClosedAssignDB(t), alwaysEligibleGate("greenhouse"))
	_, err := store.ClaimNextForNode(context.Background(), "node-1", "greenhouse")
	if err == nil {
		t.Fatal("ClaimNextForNode() on closed DB expected error, got nil")
	}
}

func TestAssignmentRepositoryService_ExtendHeartbeats_ErrInternalOnDBError(t *testing.T) {
	store := services.NewAssignmentRepositoryService(newClosedAssignDB(t), alwaysEligibleGate())
	_, _, err := store.ExtendHeartbeats(context.Background(), "node-1", []string{"q-1"})
	if err == nil {
		t.Fatal("ExtendHeartbeats() on closed DB expected error, got nil")
	}
}

func TestAssignmentRepositoryService_CompleteForNode_ErrInternalOnDBError(t *testing.T) {
	store := services.NewAssignmentRepositoryService(newClosedAssignDB(t), alwaysEligibleGate())
	err := store.CompleteForNode(context.Background(), "q-1", "node-1")
	if err == nil {
		t.Fatal("CompleteForNode() on closed DB expected error, got nil")
	}
}

func TestAssignmentRepositoryService_FailForNode_ErrInternalOnDBError(t *testing.T) {
	store := services.NewAssignmentRepositoryService(newClosedAssignDB(t), alwaysEligibleGate())
	err := store.FailForNode(context.Background(), "q-1", "node-1")
	if err == nil {
		t.Fatal("FailForNode() on closed DB expected error, got nil")
	}
}

func TestAssignmentRepositoryService_ResetStuckForNodes_ErrInternalOnDBError(t *testing.T) {
	store := services.NewAssignmentRepositoryService(newClosedAssignDB(t), alwaysEligibleGate())
	_, err := store.ResetStuckForNodes(context.Background(), 5*time.Minute)
	if err == nil {
		t.Fatal("ResetStuckForNodes() on closed DB expected error, got nil")
	}
}

// ClaimNextForNode with empty adapterType must NOT include "AND adapter_type = ?" in the WHERE clause.
func TestAssignmentRepositoryService_ClaimNextForNode_EmptyAdapterType_SQLShape(t *testing.T) {
	db, cap := newAssignDryRunDB(t)
	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate())

	_, _ = store.ClaimNextForNode(context.Background(), "node-1", "")

	stmts := cap.statements()
	if len(stmts) == 0 {
		t.Fatal("ClaimNextForNode (empty adapter): no SQL emitted")
	}
	sql := strings.ToUpper(stmts[0])
	if strings.Contains(sql, "AND ADAPTER_TYPE") {
		t.Errorf("ClaimNextForNode (empty adapter) SQL must not filter on adapter_type in WHERE:\n%s", stmts[0])
	}
}

// ClaimNextForNode with empty adapterType must propagate DB errors.
func TestAssignmentRepositoryService_ClaimNextForNode_EmptyAdapterType_ErrInternalOnDBError(t *testing.T) {
	store := services.NewAssignmentRepositoryService(newClosedAssignDB(t), alwaysEligibleGate())
	_, err := store.ClaimNextForNode(context.Background(), "node-1", "")
	if err == nil {
		t.Fatal("ClaimNextForNode (empty adapter, closed DB) expected error, got nil")
	}
}

// --- ClaimNextForNode: real SQLite exec, delay-gate + claim-race behavior ---

// Empty queue must return (nil, nil) — not an error.
func TestAssignmentRepositoryService_ClaimNextForNode_EmptyQueue_ReturnsNilNil(t *testing.T) {
	db := newAssignExecDB(t)
	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate("greenhouse"))

	item, err := store.ClaimNextForNode(context.Background(), "node-1", "")
	if err != nil {
		t.Fatalf("ClaimNextForNode() error = %v, want nil", err)
	}
	if item != nil {
		t.Errorf("ClaimNextForNode() = %+v, want nil (empty queue)", item)
	}
}

// A single eligible candidate is claimed: status flips to running, locked_by
// is stamped, and the returned item matches the row.
func TestAssignmentRepositoryService_ClaimNextForNode_ClaimsEligibleRow(t *testing.T) {
	db := newAssignExecDB(t)
	seedQueueItemFull(t, db, "q-1", "queued", "", "greenhouse", 0)
	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate("greenhouse"))

	item, err := store.ClaimNextForNode(context.Background(), "node-1", "")
	if err != nil {
		t.Fatalf("ClaimNextForNode() error = %v", err)
	}
	if item == nil || item.ID != "q-1" {
		t.Fatalf("ClaimNextForNode() = %+v, want q-1", item)
	}

	var status, lockedBy string
	if err := db.Raw("SELECT status, locked_by FROM company_scan_queue WHERE id = 'q-1'").Row().Scan(&status, &lockedBy); err != nil {
		t.Fatalf("read back row: %v", err)
	}
	if status != "running" {
		t.Errorf("status = %q, want running", status)
	}
	if lockedBy != "node-1" {
		t.Errorf("locked_by = %q, want node-1", lockedBy)
	}
}

// A queued row belonging to an auto_paused company must never be claimed,
// even when it is the only (or highest-priority) candidate — the scheduler's
// PurgeInactive sweep is a best-effort cleanup, not the authoritative gate;
// ClaimNextForNode's join against company_scan_settings is.
func TestAssignmentRepositoryService_ClaimNextForNode_SkipsAutoPausedCompany(t *testing.T) {
	db := newAssignExecDB(t)
	seedQueueItemForCompany(t, db, "q-paused", "co-paused", "greenhouse", 100, true, true)
	seedQueueItemForCompany(t, db, "q-active", "co-active", "greenhouse", 0, true, false)

	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate("greenhouse"))
	item, err := store.ClaimNextForNode(context.Background(), "node-1", "")
	if err != nil {
		t.Fatalf("ClaimNextForNode() error = %v", err)
	}
	if item == nil || item.ID != "q-active" {
		t.Fatalf("ClaimNextForNode() = %+v, want q-active (q-paused's company is auto_paused, must be skipped)", item)
	}

	var pausedStatus string
	if err := db.Raw("SELECT status FROM company_scan_queue WHERE id = 'q-paused'").Row().Scan(&pausedStatus); err != nil {
		t.Fatalf("read back q-paused: %v", err)
	}
	if pausedStatus != "queued" {
		t.Errorf("q-paused status = %q, want queued (must remain untouched)", pausedStatus)
	}
}

// A queued row belonging to a deactivated (active=false) company must never
// be claimed — mirrors the auto_paused gate above for the other half of the
// PurgeInactive eligibility check.
func TestAssignmentRepositoryService_ClaimNextForNode_SkipsInactiveCompany(t *testing.T) {
	db := newAssignExecDB(t)
	seedQueueItemForCompany(t, db, "q-inactive", "co-inactive", "greenhouse", 100, false, false)

	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate("greenhouse"))
	item, err := store.ClaimNextForNode(context.Background(), "node-1", "")
	if err != nil {
		t.Fatalf("ClaimNextForNode() error = %v", err)
	}
	if item != nil {
		t.Errorf("ClaimNextForNode() = %+v, want nil (only candidate belongs to an inactive company)", item)
	}
}

// When the highest-priority candidates' adapter types are on cooldown, the
// walk must skip them and claim the first candidate that clears the gate —
// proving the batch skip-and-continue behavior.
func TestAssignmentRepositoryService_ClaimNextForNode_SkipsCooldownCandidates(t *testing.T) {
	db := newAssignExecDB(t)
	// Highest priority first per ORDER BY priority DESC, queued_at ASC.
	seedQueueItemFull(t, db, "q-cooldown-1", "queued", "", "cooldown-adapter", 30)
	seedQueueItemFull(t, db, "q-cooldown-2", "queued", "", "cooldown-adapter", 20)
	seedQueueItemFull(t, db, "q-eligible", "queued", "", "eligible-adapter", 10)

	gate := services.NewAdapterDelayGate(&fakeAtsService{delays: map[string]int{
		"cooldown-adapter": 3600,
		"eligible-adapter": 0,
	}})
	// Pre-reserve cooldown-adapter so both of its candidates are ineligible
	// for the whole window of this test.
	if ok, err := gate.TryReserve(context.Background(), "cooldown-adapter"); err != nil || !ok {
		t.Fatalf("pre-reserve cooldown-adapter = (%v, %v), want (true, nil)", ok, err)
	}

	store := services.NewAssignmentRepositoryService(db, gate)
	item, err := store.ClaimNextForNode(context.Background(), "node-1", "")
	if err != nil {
		t.Fatalf("ClaimNextForNode() error = %v", err)
	}
	if item == nil || item.ID != "q-eligible" {
		t.Fatalf("ClaimNextForNode() = %+v, want q-eligible (both cooldown-adapter rows must be skipped)", item)
	}
}

// When every candidate's adapter type is on cooldown, ClaimNextForNode must
// return (nil, nil) — not an error — leaving all rows untouched.
func TestAssignmentRepositoryService_ClaimNextForNode_AllOnCooldown_ReturnsNilNil(t *testing.T) {
	db := newAssignExecDB(t)
	seedQueueItemFull(t, db, "q-1", "queued", "", "greenhouse", 0)
	seedQueueItemFull(t, db, "q-2", "queued", "", "greenhouse", 0)

	gate := services.NewAdapterDelayGate(&fakeAtsService{delays: map[string]int{"greenhouse": 3600}})
	if ok, err := gate.TryReserve(context.Background(), "greenhouse"); err != nil || !ok {
		t.Fatalf("pre-reserve greenhouse = (%v, %v), want (true, nil)", ok, err)
	}

	store := services.NewAssignmentRepositoryService(db, gate)
	item, err := store.ClaimNextForNode(context.Background(), "node-1", "")
	if err != nil {
		t.Fatalf("ClaimNextForNode() error = %v, want nil", err)
	}
	if item != nil {
		t.Errorf("ClaimNextForNode() = %+v, want nil (every candidate on cooldown)", item)
	}

	var stillQueued int64
	db.Raw("SELECT COUNT(*) FROM company_scan_queue WHERE status = 'queued'").Scan(&stillQueued)
	if stillQueued != 2 {
		t.Errorf("rows still queued = %d, want 2 (nothing should have been claimed)", stillQueued)
	}
}

// adapterType filtering must still be respected: a queued row for a
// different adapter type must never be claimed when a specific adapterType
// is requested, even though it would otherwise be the only candidate.
func TestAssignmentRepositoryService_ClaimNextForNode_RespectsAdapterTypeFilter(t *testing.T) {
	db := newAssignExecDB(t)
	seedQueueItemFull(t, db, "q-other", "queued", "", "lever", 100)
	seedQueueItemFull(t, db, "q-match", "queued", "", "greenhouse", 0)

	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate("greenhouse", "lever"))
	item, err := store.ClaimNextForNode(context.Background(), "node-1", "greenhouse")
	if err != nil {
		t.Fatalf("ClaimNextForNode() error = %v", err)
	}
	if item == nil || item.ID != "q-match" {
		t.Fatalf("ClaimNextForNode(adapterType=greenhouse) = %+v, want q-match", item)
	}
}

// adapterType filtering: when no row matches the requested adapter type,
// ClaimNextForNode returns (nil, nil), never claiming an unrelated row.
func TestAssignmentRepositoryService_ClaimNextForNode_AdapterTypeFilter_NoMatch_ReturnsNilNil(t *testing.T) {
	db := newAssignExecDB(t)
	seedQueueItemFull(t, db, "q-other", "queued", "", "lever", 0)

	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate("greenhouse", "lever"))
	item, err := store.ClaimNextForNode(context.Background(), "node-1", "greenhouse")
	if err != nil {
		t.Fatalf("ClaimNextForNode() error = %v, want nil", err)
	}
	if item != nil {
		t.Errorf("ClaimNextForNode(adapterType=greenhouse) = %+v, want nil (no greenhouse rows queued)", item)
	}
}

// --- ClaimNextForNode: lost DB claim race falls through to next candidate ---

// raceOnceLogger is a gormlogger.Interface that, the first time it observes
// the candidate-listing SELECT (identified by its distinctive ORDER BY
// clause — the per-row targeted UPDATE has no ORDER BY), synchronously runs a
// side-effect UPDATE against targetID via a second *sql.DB handle on the same
// shared-cache SQLite database. This deterministically simulates "another
// claimant won the race for this row" in the window between
// listClaimCandidates' SELECT and the subsequent per-row UPDATE — without
// relying on real goroutine timing.
type raceOnceLogger struct {
	gormlogger.Interface
	mu       sync.Mutex
	fired    bool
	sideDB   *sql.DB
	targetID string
}

func (l *raceOnceLogger) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sqlText, _ := fc()
	upper := strings.ToUpper(sqlText)
	isCandidateSelect := strings.Contains(upper, "FROM COMPANY_SCAN_QUEUE") &&
		strings.Contains(upper, "ORDER BY Q.PRIORITY DESC, Q.QUEUED_AT ASC") &&
		!strings.Contains(upper, "UPDATE")

	l.mu.Lock()
	shouldFire := isCandidateSelect && !l.fired
	if shouldFire {
		l.fired = true
	}
	l.mu.Unlock()

	if shouldFire {
		if _, err := l.sideDB.Exec(`UPDATE company_scan_queue SET status = 'running' WHERE id = ?`, l.targetID); err != nil {
			panic("raceOnceLogger side-effect update failed: " + err.Error())
		}
	}
}

func TestAssignmentRepositoryService_ClaimNextForNode_LostClaimRaceFallsToNextCandidate(t *testing.T) {
	db := newAssignExecDBShared(t)
	// q-1 sorts first (higher priority); it will be claimed out from under
	// the query by the injected side effect. q-2 is the fallback.
	seedQueueItemFull(t, db, "q-1", "queued", "", "greenhouse", 10)
	seedQueueItemFull(t, db, "q-2", "queued", "", "greenhouse", 5)

	rawDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	db.Logger = &raceOnceLogger{sideDB: rawDB, targetID: "q-1"}

	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate("greenhouse"))
	item, err := store.ClaimNextForNode(context.Background(), "node-1", "")
	if err != nil {
		t.Fatalf("ClaimNextForNode() error = %v", err)
	}
	if item == nil {
		t.Fatal("ClaimNextForNode() = nil, want q-2 (q-1 lost the race, should fall through)")
	}
	if item.ID != "q-2" {
		t.Errorf("ClaimNextForNode() claimed %q, want q-2 (q-1 should have lost the claim race)", item.ID)
	}

	var q1Status string
	if err := db.Raw("SELECT status FROM company_scan_queue WHERE id = 'q-1'").Row().Scan(&q1Status); err != nil {
		t.Fatalf("read back q-1: %v", err)
	}
	if q1Status != "running" {
		t.Errorf("q-1 status = %q, want running (claimed by the injected side effect, not by our store)", q1Status)
	}
}

// closeOnceLogger closes sideDB the first time it observes the
// candidate-listing SELECT — simulating a genuine DB failure (as opposed to
// a lost claim race) between listClaimCandidates and the per-row UPDATE. A
// real error here must propagate immediately, not be swallowed as "try the
// next candidate".
type closeOnceLogger struct {
	gormlogger.Interface
	mu     sync.Mutex
	fired  bool
	sideDB *sql.DB
}

func (l *closeOnceLogger) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sqlText, _ := fc()
	upper := strings.ToUpper(sqlText)
	isCandidateSelect := strings.Contains(upper, "FROM COMPANY_SCAN_QUEUE") &&
		strings.Contains(upper, "ORDER BY Q.PRIORITY DESC, Q.QUEUED_AT ASC") &&
		!strings.Contains(upper, "UPDATE")

	l.mu.Lock()
	shouldFire := isCandidateSelect && !l.fired
	if shouldFire {
		l.fired = true
	}
	l.mu.Unlock()

	if shouldFire {
		_ = l.sideDB.Close()
	}
}

func TestAssignmentRepositoryService_ClaimNextForNode_DBErrorDuringClaim_PropagatesError(t *testing.T) {
	db := newAssignExecDB(t)
	seedQueueItemFull(t, db, "q-1", "queued", "", "greenhouse", 0)

	rawDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	db.Logger = &closeOnceLogger{sideDB: rawDB}

	store := services.NewAssignmentRepositoryService(db, alwaysEligibleGate("greenhouse"))
	item, err := store.ClaimNextForNode(context.Background(), "node-1", "")
	if err == nil {
		t.Fatal("ClaimNextForNode() expected error when the claim UPDATE hits a closed connection, got nil")
	}
	if item != nil {
		t.Errorf("ClaimNextForNode() = %+v, want nil alongside the propagated error", item)
	}
}
