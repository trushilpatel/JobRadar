package tests

import (
	"context"
	"testing"

	"github.com/trushilpatel/jobradar/service/crawlernode/dto"
	"github.com/trushilpatel/jobradar/service/crawlernode/services"
)

// mockLedgerStore implements LedgerStore.
type mockLedgerStore struct {
	hasSeenFn      func(ctx context.Context, jobIDs []string) (map[string]bool, error)
	markSeenFn     func(ctx context.Context, jobIDs []string, companyID, reason string) error
	markRemovedFn  func(ctx context.Context, jobIDs []string) error
	getActiveIDsFn func(ctx context.Context, companyID string) ([]string, error)
}

func (m *mockLedgerStore) HasSeenBatch(ctx context.Context, jobIDs []string) (map[string]bool, error) {
	return m.hasSeenFn(ctx, jobIDs)
}
func (m *mockLedgerStore) MarkSeenBatch(ctx context.Context, jobIDs []string, companyID, reason string) error {
	return m.markSeenFn(ctx, jobIDs, companyID, reason)
}
func (m *mockLedgerStore) MarkRemovedBatch(ctx context.Context, jobIDs []string) error {
	return m.markRemovedFn(ctx, jobIDs)
}
func (m *mockLedgerStore) GetActiveJobIDsByCompany(ctx context.Context, companyID string) ([]string, error) {
	return m.getActiveIDsFn(ctx, companyID)
}

var _ services.LedgerStore = (*mockLedgerStore)(nil)

// mockScanLogStore implements ScanLogStore.
type mockScanLogStore struct {
	recordFn func(ctx context.Context, companyID string, r dto.ScanResult) error
}

func (m *mockScanLogStore) RecordScanResult(ctx context.Context, companyID string, r dto.ScanResult) error {
	return m.recordFn(ctx, companyID, r)
}

var _ services.ScanLogStore = (*mockScanLogStore)(nil)

// --- LedgerStore.HasSeenBatch ---

func TestLedgerStore_HasSeenBatch_ReturnsMap(t *testing.T) {
	store := &mockLedgerStore{
		hasSeenFn: func(_ context.Context, ids []string) (map[string]bool, error) {
			result := make(map[string]bool)
			for _, id := range ids {
				result[id] = id == "gh-123"
			}
			return result, nil
		},
	}
	got, err := store.HasSeenBatch(context.Background(), []string{"gh-123", "gh-456"})
	if err != nil {
		t.Fatalf("HasSeenBatch() error = %v", err)
	}
	if !got["gh-123"] {
		t.Error("HasSeenBatch() gh-123 should be seen=true")
	}
	if got["gh-456"] {
		t.Error("HasSeenBatch() gh-456 should be seen=false")
	}
}

// Empty slice must return empty map, no DB call.
func TestLedgerStore_HasSeenBatch_EmptySliceReturnsEmptyMap(t *testing.T) {
	store := &mockLedgerStore{
		hasSeenFn: func(_ context.Context, _ []string) (map[string]bool, error) {
			return map[string]bool{}, nil
		},
	}
	got, err := store.HasSeenBatch(context.Background(), []string{})
	if err != nil {
		t.Fatalf("HasSeenBatch([]) error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("HasSeenBatch([]) len = %d, want 0", len(got))
	}
}

// --- LedgerStore.MarkSeenBatch ---

// MarkSeenBatch is idempotent via ON CONFLICT DO NOTHING — duplicate IDs within the same call are safe.
func TestLedgerStore_MarkSeenBatch_Idempotent(t *testing.T) {
	calls := 0
	store := &mockLedgerStore{
		markSeenFn: func(_ context.Context, ids []string, companyID, reason string) error {
			calls++
			if companyID != "co-1" || reason != "fetch" {
				t.Errorf("MarkSeenBatch(co=%q, reason=%q)", companyID, reason)
			}
			return nil
		},
	}
	if err := store.MarkSeenBatch(context.Background(), []string{"gh-1", "gh-1"}, "co-1", "fetch"); err != nil {
		t.Fatalf("MarkSeenBatch() error = %v", err)
	}
	if calls != 1 {
		t.Errorf("MarkSeenBatch() called %d times, want 1", calls)
	}
}

// MarkSeenBatch must propagate errors (partial-batch DB failure must block NATS publish).
func TestLedgerStore_MarkSeenBatch_PropagatesError(t *testing.T) {
	store := &mockLedgerStore{
		markSeenFn: func(_ context.Context, _ []string, _, _ string) error {
			return errSentinel
		},
	}
	err := store.MarkSeenBatch(context.Background(), []string{"gh-1"}, "co-1", "fetch")
	if err == nil {
		t.Fatal("MarkSeenBatch() expected error, got nil")
	}
}

var errSentinel = errMsg("sentinel error")

type errMsg string

func (e errMsg) Error() string { return string(e) }

// --- LedgerStore.MarkRemovedBatch ---

func TestLedgerStore_MarkRemovedBatch_Success(t *testing.T) {
	called := false
	store := &mockLedgerStore{
		markRemovedFn: func(_ context.Context, ids []string) error {
			called = true
			if len(ids) != 2 {
				t.Errorf("MarkRemovedBatch len = %d, want 2", len(ids))
			}
			return nil
		},
	}
	if err := store.MarkRemovedBatch(context.Background(), []string{"gh-1", "gh-2"}); err != nil {
		t.Fatalf("MarkRemovedBatch() error = %v", err)
	}
	if !called {
		t.Error("MarkRemovedBatch: store not called")
	}
}

// MarkRemovedBatch on IDs not in ledger is a no-op — not an error.
func TestLedgerStore_MarkRemovedBatch_NoOpOnUnknownIDs(t *testing.T) {
	store := &mockLedgerStore{
		markRemovedFn: func(_ context.Context, _ []string) error {
			return nil // 0 rows affected is fine
		},
	}
	if err := store.MarkRemovedBatch(context.Background(), []string{"never-seen"}); err != nil {
		t.Fatalf("MarkRemovedBatch(unknown) error = %v, want nil", err)
	}
}

// --- LedgerStore.GetActiveJobIDsByCompany ---

func TestLedgerStore_GetActiveJobIDsByCompany_ReturnsIDs(t *testing.T) {
	want := []string{"gh-1", "gh-2", "gh-3"}
	store := &mockLedgerStore{
		getActiveIDsFn: func(_ context.Context, companyID string) ([]string, error) {
			if companyID != "co-1" {
				t.Errorf("GetActiveJobIDsByCompany companyID = %q, want co-1", companyID)
			}
			return want, nil
		},
	}
	got, err := store.GetActiveJobIDsByCompany(context.Background(), "co-1")
	if err != nil {
		t.Fatalf("GetActiveJobIDsByCompany() error = %v", err)
	}
	if len(got) != len(want) {
		t.Errorf("GetActiveJobIDsByCompany() len = %d, want %d", len(got), len(want))
	}
}

func TestLedgerStore_GetActiveJobIDsByCompany_ReturnsEmptyOnNoJobs(t *testing.T) {
	store := &mockLedgerStore{
		getActiveIDsFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{}, nil
		},
	}
	got, err := store.GetActiveJobIDsByCompany(context.Background(), "co-empty")
	if err != nil {
		t.Fatalf("GetActiveJobIDsByCompany() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetActiveJobIDsByCompany() len = %d, want 0", len(got))
	}
}

// --- ScanLogStore.RecordScanResult ---

func TestScanLogStore_RecordScanResult_Success(t *testing.T) {
	called := false
	store := &mockScanLogStore{
		recordFn: func(_ context.Context, companyID string, r dto.ScanResult) error {
			called = true
			if companyID != "co-1" {
				t.Errorf("RecordScanResult companyID = %q, want co-1", companyID)
			}
			if r.FetchedCount != 5 || r.NewCount != 2 {
				t.Errorf("RecordScanResult result = %+v", r)
			}
			return nil
		},
	}
	err := store.RecordScanResult(context.Background(), "co-1", dto.ScanResult{FetchedCount: 5, NewCount: 2, DurationMs: 1234})
	if err != nil {
		t.Fatalf("RecordScanResult() error = %v", err)
	}
	if !called {
		t.Error("RecordScanResult: store not called")
	}
}

func TestScanLogStore_RecordScanResult_ErrorPath(t *testing.T) {
	store := &mockScanLogStore{
		recordFn: func(_ context.Context, companyID string, r dto.ScanResult) error {
			if r.ErrMsg == "" {
				t.Error("RecordScanResult: expected ErrMsg to be set on error path")
			}
			return nil
		},
	}
	err := store.RecordScanResult(context.Background(), "co-1", dto.ScanResult{
		ErrMsg: "adapter timeout",
	})
	if err != nil {
		t.Fatalf("RecordScanResult(error) error = %v", err)
	}
}
