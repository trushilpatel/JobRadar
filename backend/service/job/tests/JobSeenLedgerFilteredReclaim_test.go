package services_test

import (
	"context"
	"testing"
	"time"

	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// --------------------------------------------------------------------------
// AC-4: ReclaimStuckFilteredRows store-layer tests
//
// The SQL uses a plain `first_seen_at < ?` comparison — the cutoff is
// computed in Go by the caller. SQLite can execute this directly, so the
// success path (RowsAffected > 0) is reachable in unit tests without Postgres.
//
// Helpers seedLedgerOld and seedLedgerRemoved live in JobSeenLedgerReclaim_test.go.
// --------------------------------------------------------------------------

// TestSeenLedger_ReclaimStuckFilteredRows_NoRows verifies the zero-rows
// success path: empty table → (0, nil).
func TestSeenLedger_ReclaimStuckFilteredRows_NoRows(t *testing.T) {
	t.Parallel()
	store := jobsvc.NewSeenLedgerStore(newJobTestDB(t))

	n, err := store.ReclaimStuckFilteredRows(context.Background(), time.Now().Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 rows reclaimed, got %d", n)
	}
}

// TestSeenLedger_ReclaimStuckFilteredRows_SkipsNonFiltered verifies that
// non-filtered rows (reason='fetch', reason='retry') are not deleted.
func TestSeenLedger_ReclaimStuckFilteredRows_SkipsNonFiltered(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedLedgerOld(t, db, "p1", "c1", "fetch")
	seedLedgerOld(t, db, "p2", "c1", "retry")
	store := jobsvc.NewSeenLedgerStore(db)

	n, err := store.ReclaimStuckFilteredRows(context.Background(), time.Now().Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 rows reclaimed (no filtered rows), got %d", n)
	}
}

// TestSeenLedger_ReclaimStuckFilteredRows_DeletesOldRow verifies the success
// path: an old reason='filtered' row is deleted and RowsAffected=1.
func TestSeenLedger_ReclaimStuckFilteredRows_DeletesOldRow(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedLedgerOld(t, db, "p1", "c1", "filtered") // first_seen_at = 2h ago
	store := jobsvc.NewSeenLedgerStore(db)

	n, err := store.ReclaimStuckFilteredRows(context.Background(), time.Now().Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 row deleted, got %d", n)
	}
}

// TestSeenLedger_ReclaimStuckFilteredRows_RespectsLimit verifies that the
// limit argument is honoured: with 2 eligible rows and limit=1, only 1 is deleted.
func TestSeenLedger_ReclaimStuckFilteredRows_RespectsLimit(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedLedgerOld(t, db, "p1", "c1", "filtered")
	seedLedgerOld(t, db, "p2", "c1", "filtered")
	store := jobsvc.NewSeenLedgerStore(db)

	n, err := store.ReclaimStuckFilteredRows(context.Background(), time.Now().Add(-time.Hour), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 row deleted (limit=1), got %d", n)
	}
}

// TestSeenLedger_ReclaimStuckFilteredRows_RemovedAtIsNotNull_Included verifies
// that rows with removed_at IS NOT NULL are still deleted — no removed_at filter.
func TestSeenLedger_ReclaimStuckFilteredRows_RemovedAtIsNotNull_Included(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	// seedLedgerRemoved marks removed_at; we also need to backdate first_seen_at.
	seedLedgerRemoved(t, db, "p1", "c1", "filtered")
	if err := db.Model(&jobmodel.JobSeenLedger{}).Where("job_id = ?", "p1").
		Update("first_seen_at", time.Now().Add(-2*time.Hour)).Error; err != nil {
		t.Fatalf("backdate first_seen_at: %v", err)
	}
	store := jobsvc.NewSeenLedgerStore(db)

	n, err := store.ReclaimStuckFilteredRows(context.Background(), time.Now().Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 (removed row still deleted), got %d", n)
	}
}

// TestSeenLedger_ReclaimStuckFilteredRows_SkipsRecentRow verifies that a row
// with first_seen_at = now is not deleted when olderThan is 1 hour ago.
func TestSeenLedger_ReclaimStuckFilteredRows_SkipsRecentRow(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedLedger(t, db, "p1", "c1", "filtered") // first_seen_at = now
	store := jobsvc.NewSeenLedgerStore(db)

	n, err := store.ReclaimStuckFilteredRows(context.Background(), time.Now().Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 (row too recent), got %d", n)
	}
}

// TestSeenLedger_ReclaimStuckFilteredRows_DBError verifies the error branch:
// when the database is unavailable, ReclaimStuckFilteredRows returns (0, err).
func TestSeenLedger_ReclaimStuckFilteredRows_DBError(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	_ = sqlDB.Close()

	store := jobsvc.NewSeenLedgerStore(db)
	n, closeErr := store.ReclaimStuckFilteredRows(context.Background(), time.Now(), 100)
	if closeErr == nil {
		t.Fatal("expected error from closed DB")
	}
	if n != 0 {
		t.Errorf("expected 0 rows on error, got %d", n)
	}
}
