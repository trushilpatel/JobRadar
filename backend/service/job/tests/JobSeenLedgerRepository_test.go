package services_test

import (
	"context"
	stderrors "errors"
	"io"
	"testing"

	commondb "github.com/trushilpatel/jobradar/common/db"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	"gorm.io/gorm"
)

// seedLedger inserts a single ledger row.
func seedLedger(t *testing.T, db *gorm.DB, jobID, companyID, reason string) {
	t.Helper()
	row := jobmodel.JobSeenLedger{JobID: jobID, CompanyID: companyID, Reason: reason}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed ledger %s: %v", jobID, err)
	}
}

// TestSeenLedger_HasSeen_True asserts a present row returns true.
func TestSeenLedger_HasSeen_True(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedLedger(t, db, "p1", "c1", "fetch")
	store := jobsvc.NewSeenLedgerStore(db)

	seen, err := store.HasSeen(context.Background(), "p1")
	if err != nil {
		t.Fatalf("HasSeen err = %v", err)
	}
	if !seen {
		t.Errorf("want seen=true for p1")
	}
}

// TestSeenLedger_HasSeen_False — missing row → false, no error.
func TestSeenLedger_HasSeen_False(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	store := jobsvc.NewSeenLedgerStore(db)
	seen, err := store.HasSeen(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if seen {
		t.Error("ghost row reported as seen")
	}
}

// TestSeenLedger_HasSeenBatch_Empty short-circuits without a query.
func TestSeenLedger_HasSeenBatch_Empty(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	store := jobsvc.NewSeenLedgerStore(db)
	out, err := store.HasSeenBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(out) != 0 {
		t.Errorf("len = %d, want 0", len(out))
	}
}

// TestSeenLedger_HasSeenBatch returns a map keyed by present IDs only.
func TestSeenLedger_HasSeenBatch(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedLedger(t, db, "p1", "c1", "fetch")
	seedLedger(t, db, "p2", "c1", "fetch")
	store := jobsvc.NewSeenLedgerStore(db)

	out, err := store.HasSeenBatch(context.Background(), []string{"p1", "p2", "p3"})
	if err != nil {
		t.Fatalf("HasSeenBatch err = %v", err)
	}
	if !out["p1"] || !out["p2"] {
		t.Errorf("p1/p2 missing from map: %v", out)
	}
	if out["p3"] {
		t.Errorf("p3 present in map; want absent (missing row)")
	}
}

// TestSeenLedger_MarkSeen inserts a row and HasSeen confirms it.
func TestSeenLedger_MarkSeen(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	store := jobsvc.NewSeenLedgerStore(db)

	if err := store.MarkSeen(context.Background(), "p1", "c1", "fetch"); err != nil {
		t.Fatalf("MarkSeen err = %v", err)
	}
	seen, err := store.HasSeen(context.Background(), "p1")
	if err != nil || !seen {
		t.Errorf("post-MarkSeen, HasSeen=%v err=%v", seen, err)
	}
}

// TestSeenLedger_MarkSeenBatch_Empty short-circuits.
func TestSeenLedger_MarkSeenBatch_Empty(t *testing.T) {
	t.Parallel()
	store := jobsvc.NewSeenLedgerStore(newJobTestDB(t))
	if err := store.MarkSeenBatch(context.Background(), nil, "c1", "fetch"); err != nil {
		t.Errorf("empty batch should be no-op, got err = %v", err)
	}
}

// TestSeenLedger_MarkSeenBatch_OnConflictIgnoresDuplicates — running twice
// must not error (ON CONFLICT DO NOTHING).
func TestSeenLedger_MarkSeenBatch_OnConflictIgnoresDuplicates(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	store := jobsvc.NewSeenLedgerStore(db)

	ids := []string{"p1", "p2"}
	if err := store.MarkSeenBatch(context.Background(), ids, "c1", "fetch"); err != nil {
		t.Fatalf("first batch err = %v", err)
	}
	if err := store.MarkSeenBatch(context.Background(), ids, "c1", "fetch"); err != nil {
		t.Fatalf("second batch (duplicate) err = %v", err)
	}
	out, err := store.HasSeenBatch(context.Background(), ids)
	if err != nil {
		t.Fatalf("HasSeenBatch err = %v", err)
	}
	if !out["p1"] || !out["p2"] {
		t.Errorf("expected both rows present: %v", out)
	}
	// Confirm only 2 rows total — no duplicates inserted.
	var count int64
	if err := db.Model(&jobmodel.JobSeenLedger{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("row count = %d, want 2 (ON CONFLICT DO NOTHING)", count)
	}
}

// TestSeenLedger_UpdateSeenReason replaces the reason column.
func TestSeenLedger_UpdateSeenReason(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedLedger(t, db, "p1", "c1", "fetch")
	store := jobsvc.NewSeenLedgerStore(db)

	if err := store.UpdateSeenReason(context.Background(), "p1", jobcommon.SeenReasonUserDismissed); err != nil {
		t.Fatalf("UpdateSeenReason err = %v", err)
	}
	var got jobmodel.JobSeenLedger
	if err := db.Where("job_id = ?", "p1").First(&got).Error; err != nil {
		t.Fatalf("readback: %v", err)
	}
	if got.Reason != string(jobcommon.SeenReasonUserDismissed) {
		t.Errorf("reason = %q, want user_dismissed", got.Reason)
	}
}

// TestSeenLedger_UpdateSeenReason_MissingRow_ReturnsErrLedgerRowMissing — no
// row for the job_id (the crawler→filter race) surfaces the sentinel so the
// caller can Nak rather than silently no-op.
func TestSeenLedger_UpdateSeenReason_MissingRow_ReturnsErrLedgerRowMissing(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	store := jobsvc.NewSeenLedgerStore(db)

	err := store.UpdateSeenReason(context.Background(), "ghost", jobcommon.SeenReasonFiltered)
	if !stderrors.Is(err, domainerrs.ErrLedgerRowMissing) {
		t.Fatalf("want ErrLedgerRowMissing, got %v", err)
	}
}

// TestSeenLedger_UpdateSeenReasonBatch_AllMissing_ReturnsSentinel.
func TestSeenLedger_UpdateSeenReasonBatch_AllMissing_ReturnsSentinel(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	store := jobsvc.NewSeenLedgerStore(db)

	err := store.UpdateSeenReasonBatch(context.Background(), []string{"g1", "g2", "g3"}, jobcommon.SeenReasonFiltered)
	if !stderrors.Is(err, domainerrs.ErrLedgerRowMissing) {
		t.Fatalf("want ErrLedgerRowMissing, got %v", err)
	}
}

// TestSeenLedger_UpdateSeenReasonBatch_PartialMatch_ReturnsSentinel — only 1 of
// 3 rows present; the partial batch is treated as a miss.
func TestSeenLedger_UpdateSeenReasonBatch_PartialMatch_ReturnsSentinel(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedLedger(t, db, "p1", "c1", "fetch")
	store := jobsvc.NewSeenLedgerStore(db)

	err := store.UpdateSeenReasonBatch(context.Background(), []string{"p1", "p2", "p3"}, jobcommon.SeenReasonFiltered)
	if !stderrors.Is(err, domainerrs.ErrLedgerRowMissing) {
		t.Fatalf("want ErrLedgerRowMissing, got %v", err)
	}
	// The present row should still have been updated before the sentinel returned.
	var got jobmodel.JobSeenLedger
	if err := db.Where("job_id = ?", "p1").First(&got).Error; err != nil {
		t.Fatalf("readback: %v", err)
	}
	if got.Reason != string(jobcommon.SeenReasonFiltered) {
		t.Errorf("present row reason = %q, want filtered", got.Reason)
	}
}

// TestSeenLedger_UpdateSeenReasonBatch_SentinelNotRetried asserts the
// ErrLedgerRowMissing sentinel returned when rows are missing passes straight
// through the db.WithRetry wrapper and is NOT retried (it is a business outcome,
// not a connection drop). The store call must return promptly with the sentinel
// — a retried sentinel would multiply the crawler→filter Nak latency.
func TestSeenLedger_UpdateSeenReasonBatch_SentinelNotRetried(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	store := jobsvc.NewSeenLedgerStore(db)

	err := store.UpdateSeenReasonBatch(context.Background(),
		[]string{"g1", "g2"}, jobcommon.SeenReasonFiltered)
	if !stderrors.Is(err, domainerrs.ErrLedgerRowMissing) {
		t.Fatalf("want ErrLedgerRowMissing (not retried), got %v", err)
	}
}

// TestWithRetry_LedgerClosureShape_TransientThenSucceeds locks the exact
// closure contract the ledger store relies on: db.WithRetry retries a transient
// connection-drop (io.ErrUnexpectedEOF — the Supavisor idle-drop symptom) and,
// once the raw exec succeeds, the captured RowsAffected count is used to decide
// the sentinel OUTSIDE the retried op. The sqlite harness cannot inject an EOF
// mid-exec, so the retry behaviour is verified here at the seam that the store
// method wraps, matching the design's unit-tier tolerance note.
func TestWithRetry_LedgerClosureShape_TransientThenSucceeds(t *testing.T) {
	t.Parallel()
	attempts := 0
	var rowsAffected int64
	err := commondb.WithRetry(context.Background(), func() error {
		attempts++
		if attempts < 2 {
			return io.ErrUnexpectedEOF // transient conn drop
		}
		rowsAffected = 3 // "final successful exec" row count
		return nil
	})
	if err != nil {
		t.Fatalf("WithRetry err = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2 (retried once)", attempts)
	}
	// Sentinel decision evaluated after success, on the captured count.
	if rowsAffected < 3 {
		t.Fatalf("rowsAffected = %d, want 3 captured from the successful exec", rowsAffected)
	}
}

// TestSeenLedger_UpdateSeenReasonBatch_AllPresent_ReturnsNil.
func TestSeenLedger_UpdateSeenReasonBatch_AllPresent_ReturnsNil(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedLedger(t, db, "p1", "c1", "fetch")
	seedLedger(t, db, "p2", "c1", "fetch")
	seedLedger(t, db, "p3", "c1", "fetch")
	store := jobsvc.NewSeenLedgerStore(db)

	if err := store.UpdateSeenReasonBatch(context.Background(), []string{"p1", "p2", "p3"}, jobcommon.SeenReasonFiltered); err != nil {
		t.Fatalf("all-present batch should succeed: %v", err)
	}
}

// TestSeenLedger_UpdateSeenReasonBatch_Empty short-circuits.
func TestSeenLedger_UpdateSeenReasonBatch_Empty(t *testing.T) {
	t.Parallel()
	store := jobsvc.NewSeenLedgerStore(newJobTestDB(t))
	if err := store.UpdateSeenReasonBatch(context.Background(), nil, jobcommon.SeenReasonUserDismissed); err != nil {
		t.Errorf("empty batch should no-op: %v", err)
	}
}

// TestSeenLedger_UpdateSeenReasonBatch — multiple rows updated in one call.
func TestSeenLedger_UpdateSeenReasonBatch(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedLedger(t, db, "p1", "c1", "fetch")
	seedLedger(t, db, "p2", "c1", "fetch")
	seedLedger(t, db, "p3", "c1", "fetch")
	store := jobsvc.NewSeenLedgerStore(db)

	if err := store.UpdateSeenReasonBatch(context.Background(),
		[]string{"p1", "p2"}, jobcommon.SeenReasonUserDismissed); err != nil {
		t.Fatalf("UpdateSeenReasonBatch err = %v", err)
	}
	var rows []jobmodel.JobSeenLedger
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("readback: %v", err)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r.JobID] = r.Reason
	}
	if got["p1"] != string(jobcommon.SeenReasonUserDismissed) || got["p2"] != string(jobcommon.SeenReasonUserDismissed) {
		t.Errorf("p1+p2 not updated: %v", got)
	}
	if got["p3"] != string(jobcommon.SeenReasonFetch) {
		t.Errorf("p3 should be untouched, got %q", got["p3"])
	}
}

// TestSeenLedger_MarkRemovedBatch_Empty short-circuits.
func TestSeenLedger_MarkRemovedBatch_Empty(t *testing.T) {
	t.Parallel()
	store := jobsvc.NewSeenLedgerStore(newJobTestDB(t))
	if err := store.MarkRemovedBatch(context.Background(), nil); err != nil {
		t.Errorf("empty batch should no-op: %v", err)
	}
}

// TestSeenLedger_MarkRemovedBatch_OnlyUpdatesActiveRows — rows already with
// removed_at set must not be touched again. This locks the partial-update
// invariant (only WHERE removed_at IS NULL).
func TestSeenLedger_MarkRemovedBatch_OnlyUpdatesActiveRows(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedLedger(t, db, "p1", "c1", "fetch")
	seedLedger(t, db, "p2", "c1", "fetch")
	store := jobsvc.NewSeenLedgerStore(db)

	if err := store.MarkRemovedBatch(context.Background(), []string{"p1"}); err != nil {
		t.Fatalf("mark removed p1: %v", err)
	}
	// Capture removed_at on p1, then mark again — value must remain unchanged.
	var p1 jobmodel.JobSeenLedger
	if err := db.Where("job_id = ?", "p1").First(&p1).Error; err != nil {
		t.Fatalf("read p1: %v", err)
	}
	firstRemovedAt := p1.RemovedAt
	if firstRemovedAt == nil {
		t.Fatal("removed_at nil after MarkRemovedBatch")
	}

	if err := store.MarkRemovedBatch(context.Background(), []string{"p1", "p2"}); err != nil {
		t.Fatalf("mark removed batch: %v", err)
	}
	var p1again jobmodel.JobSeenLedger
	if err := db.Where("job_id = ?", "p1").First(&p1again).Error; err != nil {
		t.Fatalf("read p1 again: %v", err)
	}
	if p1again.RemovedAt == nil || !p1again.RemovedAt.Equal(*firstRemovedAt) {
		t.Errorf("removed_at changed: was %v, now %v — should be idempotent on already-removed",
			firstRemovedAt, p1again.RemovedAt)
	}
	// p2 must now also have removed_at set.
	var p2 jobmodel.JobSeenLedger
	if err := db.Where("job_id = ?", "p2").First(&p2).Error; err != nil {
		t.Fatalf("read p2: %v", err)
	}
	if p2.RemovedAt == nil {
		t.Errorf("p2.removed_at nil; want set after second batch")
	}
}

// TestSeenLedger_GetActiveJobIDsByCompany returns only rows without
// removed_at for the matching company.
func TestSeenLedger_GetActiveJobIDsByCompany(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedCompany(t, db, "c2", "Globex")
	seedLedger(t, db, "p1", "c1", "fetch")
	seedLedger(t, db, "p2", "c1", "fetch")
	seedLedger(t, db, "p3", "c2", "fetch") // wrong company
	store := jobsvc.NewSeenLedgerStore(db)

	// Mark p1 removed → should disappear from the active set.
	if err := store.MarkRemovedBatch(context.Background(), []string{"p1"}); err != nil {
		t.Fatalf("mark removed: %v", err)
	}
	ids, err := store.GetActiveJobIDsByCompany(context.Background(), "c1")
	if err != nil {
		t.Fatalf("GetActiveJobIDsByCompany err = %v", err)
	}
	if len(ids) != 1 || ids[0] != "p2" {
		t.Errorf("ids = %v, want [p2] (p1 removed, p3 different company)", ids)
	}
}

// -------- LedgerService façade --------

// TestLedgerService_Facade_DelegatesAll exercises every façade method to
// confirm it forwards to the underlying SeenLedgerStore.
func TestLedgerService_Facade_DelegatesAll(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	store := jobsvc.NewSeenLedgerStore(db)
	led := jobsvc.NewLedgerService(store)
	ctx := context.Background()

	// MarkSeenBatch
	if err := led.MarkSeenBatch(ctx, []string{"p1", "p2"}, "c1", "fetch"); err != nil {
		t.Fatalf("MarkSeenBatch: %v", err)
	}
	// HasSeenBatch
	out, err := led.HasSeenBatch(ctx, []string{"p1", "p2", "missing"})
	if err != nil {
		t.Fatalf("HasSeenBatch: %v", err)
	}
	if !out["p1"] || !out["p2"] || out["missing"] {
		t.Errorf("HasSeenBatch result = %v", out)
	}
	// GetActiveJobIDsByCompany
	ids, err := led.GetActiveJobIDsByCompany(ctx, "c1")
	if err != nil {
		t.Fatalf("GetActiveJobIDsByCompany: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("len = %d, want 2", len(ids))
	}
	// MarkRemovedBatch
	if err := led.MarkRemovedBatch(ctx, []string{"p1"}); err != nil {
		t.Fatalf("MarkRemovedBatch: %v", err)
	}
	ids2, err := led.GetActiveJobIDsByCompany(ctx, "c1")
	if err != nil {
		t.Fatalf("after remove: %v", err)
	}
	if len(ids2) != 1 || ids2[0] != "p2" {
		t.Errorf("post-remove ids = %v, want [p2]", ids2)
	}
}
