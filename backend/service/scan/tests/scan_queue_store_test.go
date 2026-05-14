package scan

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

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
		"FROM adapter_configs",
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

	if err := store.ResetStuck(context.Background(), 90*time.Second); err != nil {
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
