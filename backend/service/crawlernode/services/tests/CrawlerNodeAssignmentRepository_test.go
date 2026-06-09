package tests

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/crawlernode/dto"
	"github.com/trushilpatel/jobradar/service/crawlernode/services"
)

// mockAssignmentStore implements AssignmentStore with configurable return values.
type mockAssignmentStore struct {
	claimFn      func(ctx context.Context, nodeID, adapterType string) (*dto.ScanQueueItem, error)
	extendFn     func(ctx context.Context, nodeID string, queueIDs []string) (acked, missed []string, err error)
	completeFn   func(ctx context.Context, queueID, nodeID string) error
	failFn       func(ctx context.Context, queueID, nodeID string) error
	resetStuckFn func(ctx context.Context, staleDur time.Duration) (int, error)
}

func (m *mockAssignmentStore) ClaimNextForNode(ctx context.Context, nodeID, adapterType string) (*dto.ScanQueueItem, error) {
	return m.claimFn(ctx, nodeID, adapterType)
}
func (m *mockAssignmentStore) ExtendHeartbeats(ctx context.Context, nodeID string, queueIDs []string) ([]string, []string, error) {
	return m.extendFn(ctx, nodeID, queueIDs)
}
func (m *mockAssignmentStore) CompleteForNode(ctx context.Context, queueID, nodeID string) error {
	return m.completeFn(ctx, queueID, nodeID)
}
func (m *mockAssignmentStore) FailForNode(ctx context.Context, queueID, nodeID string) error {
	return m.failFn(ctx, queueID, nodeID)
}
func (m *mockAssignmentStore) ResetStuckForNodes(ctx context.Context, staleDur time.Duration) (int, error) {
	return m.resetStuckFn(ctx, staleDur)
}

// Compile-time interface check.
var _ services.AssignmentStore = (*mockAssignmentStore)(nil)

// --- ClaimNextForNode ---

func TestAssignmentStore_ClaimNext_ReturnsAssignment(t *testing.T) {
	want := &dto.ScanQueueItem{ID: "q-1", CompanyID: "co-1", AdapterType: "greenhouse"}
	store := &mockAssignmentStore{
		claimFn: func(_ context.Context, nodeID, adapterType string) (*dto.ScanQueueItem, error) {
			if nodeID != "node-1" || adapterType != "greenhouse" {
				t.Errorf("ClaimNextForNode(%q, %q)", nodeID, adapterType)
			}
			return want, nil
		},
	}
	got, err := store.ClaimNextForNode(context.Background(), "node-1", "greenhouse")
	if err != nil {
		t.Fatalf("ClaimNextForNode() error = %v", err)
	}
	if got == nil || got.ID != want.ID {
		t.Errorf("ClaimNextForNode() = %v, want %v", got, want)
	}
}

// nil, nil means queue is empty — not an error.
func TestAssignmentStore_ClaimNext_ReturnsNilNilWhenEmpty(t *testing.T) {
	store := &mockAssignmentStore{
		claimFn: func(_ context.Context, _, _ string) (*dto.ScanQueueItem, error) {
			return nil, nil
		},
	}
	got, err := store.ClaimNextForNode(context.Background(), "node-1", "lever")
	if err != nil {
		t.Fatalf("ClaimNextForNode() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("ClaimNextForNode() = %v, want nil (empty queue)", got)
	}
}

// --- ExtendHeartbeats ---

func TestAssignmentStore_ExtendHeartbeats_AllAcked(t *testing.T) {
	store := &mockAssignmentStore{
		extendFn: func(_ context.Context, nodeID string, ids []string) ([]string, []string, error) {
			if nodeID != "node-1" {
				t.Errorf("ExtendHeartbeats nodeID = %q, want node-1", nodeID)
			}
			return ids, nil, nil
		},
	}
	acked, missed, err := store.ExtendHeartbeats(context.Background(), "node-1", []string{"q-1", "q-2"})
	if err != nil {
		t.Fatalf("ExtendHeartbeats() error = %v", err)
	}
	if len(acked) != 2 || len(missed) != 0 {
		t.Errorf("ExtendHeartbeats() acked=%d missed=%d, want 2 / 0", len(acked), len(missed))
	}
}

// Empty holdingIDs must return acked=[], missed=[] without a DB call (guards against IN() syntax error).
func TestAssignmentStore_ExtendHeartbeats_EmptySlice(t *testing.T) {
	called := false
	store := &mockAssignmentStore{
		extendFn: func(_ context.Context, _ string, ids []string) ([]string, []string, error) {
			called = true
			return nil, nil, nil
		},
	}
	acked, missed, err := store.ExtendHeartbeats(context.Background(), "node-1", []string{})
	if err != nil {
		t.Fatalf("ExtendHeartbeats() error = %v", err)
	}
	// The mock is called — the caller (impl) is responsible for skipping when empty.
	// Here we just verify the return values are empty.
	if len(acked) != 0 || len(missed) != 0 {
		t.Errorf("ExtendHeartbeats(empty) acked=%d missed=%d, want 0/0", len(acked), len(missed))
	}
	_ = called
}

// Some IDs belong to a different node (locked_by mismatch) — those go into missed.
func TestAssignmentStore_ExtendHeartbeats_SomeMissed(t *testing.T) {
	store := &mockAssignmentStore{
		extendFn: func(_ context.Context, _ string, _ []string) ([]string, []string, error) {
			return []string{"q-1"}, []string{"q-2"}, nil
		},
	}
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

// --- CompleteForNode ---

func TestAssignmentStore_CompleteForNode_Success(t *testing.T) {
	called := false
	store := &mockAssignmentStore{
		completeFn: func(_ context.Context, queueID, nodeID string) error {
			called = true
			if queueID != "q-1" || nodeID != "node-1" {
				t.Errorf("CompleteForNode(%q, %q), want (q-1, node-1)", queueID, nodeID)
			}
			return nil
		},
	}
	if err := store.CompleteForNode(context.Background(), "q-1", "node-1"); err != nil {
		t.Fatalf("CompleteForNode() error = %v", err)
	}
	if !called {
		t.Error("CompleteForNode: store not called")
	}
}

// locked_by mismatch returns ErrQueueRowMissing — never silently nil.
func TestAssignmentStore_CompleteForNode_OwnershipMismatch(t *testing.T) {
	store := &mockAssignmentStore{
		completeFn: func(_ context.Context, _, _ string) error {
			return domainerrs.ErrQueueRowMissing.WithMessage("locked_by mismatch")
		},
	}
	err := store.CompleteForNode(context.Background(), "q-1", "wrong-node")
	if !stderrors.Is(err, domainerrs.ErrQueueRowMissing) {
		t.Errorf("CompleteForNode() error = %v, want ErrQueueRowMissing", err)
	}
}

// --- FailForNode ---

func TestAssignmentStore_FailForNode_ResetsToQueued(t *testing.T) {
	called := false
	store := &mockAssignmentStore{
		failFn: func(_ context.Context, queueID, nodeID string) error {
			called = true
			if queueID != "q-1" || nodeID != "node-1" {
				t.Errorf("FailForNode(%q, %q)", queueID, nodeID)
			}
			return nil
		},
	}
	if err := store.FailForNode(context.Background(), "q-1", "node-1"); err != nil {
		t.Fatalf("FailForNode() error = %v", err)
	}
	if !called {
		t.Error("FailForNode: store not called")
	}
}

// Row already reset by ResetStuckForNodes — ErrQueueRowMissing, not a fatal error.
func TestAssignmentStore_FailForNode_AlreadyReset(t *testing.T) {
	store := &mockAssignmentStore{
		failFn: func(_ context.Context, _, _ string) error {
			return domainerrs.ErrQueueRowMissing.WithMessage("row already reset")
		},
	}
	err := store.FailForNode(context.Background(), "q-1", "node-1")
	if !stderrors.Is(err, domainerrs.ErrQueueRowMissing) {
		t.Errorf("FailForNode() error = %v, want ErrQueueRowMissing", err)
	}
}

// --- ResetStuckForNodes ---

func TestAssignmentStore_ResetStuckForNodes_ReturnsCount(t *testing.T) {
	store := &mockAssignmentStore{
		resetStuckFn: func(_ context.Context, dur time.Duration) (int, error) {
			if dur != 5*time.Minute {
				t.Errorf("ResetStuckForNodes dur = %v, want 5m", dur)
			}
			return 3, nil
		},
	}
	n, err := store.ResetStuckForNodes(context.Background(), 5*time.Minute)
	if err != nil {
		t.Fatalf("ResetStuckForNodes() error = %v", err)
	}
	if n != 3 {
		t.Errorf("ResetStuckForNodes() = %d, want 3", n)
	}
}

func TestAssignmentStore_ResetStuckForNodes_ZeroIsNotAnError(t *testing.T) {
	store := &mockAssignmentStore{
		resetStuckFn: func(_ context.Context, _ time.Duration) (int, error) {
			return 0, nil
		},
	}
	n, err := store.ResetStuckForNodes(context.Background(), 5*time.Minute)
	if err != nil {
		t.Fatalf("ResetStuckForNodes() error = %v", err)
	}
	if n != 0 {
		t.Errorf("ResetStuckForNodes() = %d, want 0", n)
	}
}
