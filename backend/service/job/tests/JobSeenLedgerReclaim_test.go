package services_test

import (
	"context"
	"testing"
	"time"

	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	"gorm.io/gorm"
)

// seedLedgerOld inserts a ledger row and backdates its first_seen_at to 2 hours
// ago via a raw UPDATE. This makes the row eligible for reclaim when callers
// pass olderThan = time.Now().Add(-1h), because first_seen_at < olderThan.
func seedLedgerOld(t testing.TB, db *gorm.DB, jobID, companyID, reason string) {
	t.Helper()
	row := jobmodel.JobSeenLedger{JobID: jobID, CompanyID: companyID, Reason: reason}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed ledger %s: %v", jobID, err)
	}
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	if err := db.Model(&jobmodel.JobSeenLedger{}).Where("job_id = ?", jobID).
		Update("first_seen_at", twoHoursAgo).Error; err != nil {
		t.Fatalf("backdate first_seen_at on %s: %v", jobID, err)
	}
}

// seedLedgerRemoved inserts a ledger row and immediately sets removed_at to a
// non-NULL value. Used by tests that verify removed rows are included (not excluded).
func seedLedgerRemoved(t testing.TB, db *gorm.DB, jobID, companyID, reason string) {
	t.Helper()
	row := jobmodel.JobSeenLedger{JobID: jobID, CompanyID: companyID, Reason: reason}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed ledger %s: %v", jobID, err)
	}
	now := time.Now()
	if err := db.Model(&jobmodel.JobSeenLedger{}).Where("job_id = ?", jobID).Update("removed_at", now).Error; err != nil {
		t.Fatalf("set removed_at on %s: %v", jobID, err)
	}
}

// --------------------------------------------------------------------------
// ReclaimStuckFetchRows
//
// The SQL now uses a plain `first_seen_at < ?` comparison (no Postgres-specific
// NOW()-INTERVAL arithmetic) so the cutoff is computed in Go by the caller.
// SQLite can execute the comparison directly — both the success path (rows
// deleted, RowsAffected > 0) and the zero-rows path are reachable in unit tests.
// --------------------------------------------------------------------------

// TestSeenLedger_ReclaimStuckFetchRows_NoRows verifies the zero-rows success
// path: empty table → (0, nil).
func TestSeenLedger_ReclaimStuckFetchRows_NoRows(t *testing.T) {
	t.Parallel()
	store := jobsvc.NewSeenLedgerStore(newJobTestDB(t))

	n, err := store.ReclaimStuckFetchRows(context.Background(), time.Now().Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 rows reclaimed, got %d", n)
	}
}

// TestSeenLedger_ReclaimStuckFetchRows_SkipsNonFetch verifies that only
// reason='fetch' rows are targeted — non-fetch rows survive.
func TestSeenLedger_ReclaimStuckFetchRows_SkipsNonFetch(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedLedgerOld(t, db, "p1", "c1", "retry")
	seedLedgerOld(t, db, "p2", "c1", "filtered")
	store := jobsvc.NewSeenLedgerStore(db)

	n, err := store.ReclaimStuckFetchRows(context.Background(), time.Now().Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 rows reclaimed (no fetch rows), got %d", n)
	}
}

// TestSeenLedger_ReclaimStuckFetchRows_DeletesOldFetchRow verifies the
// success path: an old reason='fetch' row is deleted and RowsAffected=1.
func TestSeenLedger_ReclaimStuckFetchRows_DeletesOldFetchRow(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedLedgerOld(t, db, "p1", "c1", "fetch") // first_seen_at = 2h ago
	store := jobsvc.NewSeenLedgerStore(db)

	n, err := store.ReclaimStuckFetchRows(context.Background(), time.Now().Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 row deleted, got %d", n)
	}
}

// TestSeenLedger_ReclaimStuckFetchRows_SkipsRecentRow verifies that a row
// inserted just now is not deleted when olderThan is 1 hour ago.
func TestSeenLedger_ReclaimStuckFetchRows_SkipsRecentRow(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedLedger(t, db, "p1", "c1", "fetch") // first_seen_at = now
	store := jobsvc.NewSeenLedgerStore(db)

	n, err := store.ReclaimStuckFetchRows(context.Background(), time.Now().Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 (row too recent), got %d", n)
	}
}

// TestSeenLedger_ReclaimStuckFetchRows_RemovedAtIsNotNull_NotExcluded verifies
// that a removed job (removed_at IS NOT NULL) whose fetch row was never
// transitioned is still deleted — no removed_at filter in the query.
func TestSeenLedger_ReclaimStuckFetchRows_RemovedAtIsNotNull_NotExcluded(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	// seedLedgerRemoved marks removed_at; we also need to backdate first_seen_at.
	seedLedgerRemoved(t, db, "p1", "c1", "fetch")
	if err := db.Model(&jobmodel.JobSeenLedger{}).Where("job_id = ?", "p1").
		Update("first_seen_at", time.Now().Add(-2*time.Hour)).Error; err != nil {
		t.Fatalf("backdate first_seen_at: %v", err)
	}
	store := jobsvc.NewSeenLedgerStore(db)

	n, err := store.ReclaimStuckFetchRows(context.Background(), time.Now().Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 (removed row still deleted), got %d", n)
	}
}

// TestSeenLedger_ReclaimStuckFetchRows_DBError verifies the error branch:
// when the database is unavailable, ReclaimStuckFetchRows returns (0, err).
func TestSeenLedger_ReclaimStuckFetchRows_DBError(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	_ = sqlDB.Close()

	store := jobsvc.NewSeenLedgerStore(db)
	n, closeErr := store.ReclaimStuckFetchRows(context.Background(), time.Now(), 100)
	if closeErr == nil {
		t.Fatal("expected error from closed DB")
	}
	if n != 0 {
		t.Errorf("expected 0 rows on error, got %d", n)
	}
}
