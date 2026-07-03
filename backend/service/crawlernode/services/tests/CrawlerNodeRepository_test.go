package tests

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	crawlernodemodel "github.com/trushilpatel/jobradar/service/crawlernode/models"
	"github.com/trushilpatel/jobradar/service/crawlernode/services"
)

// mockNodeStore implements NodeStore with configurable return values.
type mockNodeStore struct {
	createFn         func(ctx context.Context, node crawlernodemodel.CrawlerNode) (*crawlernodemodel.CrawlerNode, error)
	getByPrefixFn    func(ctx context.Context, prefix string) ([]*crawlernodemodel.CrawlerNode, error)
	updateLastSeenFn func(ctx context.Context, nodeID string) error
	setStatusFn      func(ctx context.Context, nodeID, status string) error
	listFn           func(ctx context.Context, status string) ([]crawlernodemodel.CrawlerNode, error)
	retireFn         func(ctx context.Context, nodeID string) error
}

func (m *mockNodeStore) Create(ctx context.Context, node crawlernodemodel.CrawlerNode) (*crawlernodemodel.CrawlerNode, error) {
	return m.createFn(ctx, node)
}
func (m *mockNodeStore) GetByTokenPrefix(ctx context.Context, prefix string) ([]*crawlernodemodel.CrawlerNode, error) {
	return m.getByPrefixFn(ctx, prefix)
}
func (m *mockNodeStore) UpdateLastSeen(ctx context.Context, nodeID string) error {
	return m.updateLastSeenFn(ctx, nodeID)
}
func (m *mockNodeStore) SetStatus(ctx context.Context, nodeID, status string) error {
	return m.setStatusFn(ctx, nodeID, status)
}
func (m *mockNodeStore) List(ctx context.Context, status string) ([]crawlernodemodel.CrawlerNode, error) {
	return m.listFn(ctx, status)
}
func (m *mockNodeStore) Retire(ctx context.Context, nodeID string) error {
	return m.retireFn(ctx, nodeID)
}
func (m *mockNodeStore) CountOnline(context.Context) (int, error) { return 0, nil }

// Compile-time interface check.
var _ services.NodeStore = (*mockNodeStore)(nil)

// --- Create ---

func TestNodeStore_Create_ReturnsCreatedNode(t *testing.T) {
	want := &crawlernodemodel.CrawlerNode{ID: "uuid-1", Name: "prod-node"}
	store := &mockNodeStore{
		createFn: func(_ context.Context, n crawlernodemodel.CrawlerNode) (*crawlernodemodel.CrawlerNode, error) {
			return want, nil
		},
	}
	got, err := store.Create(context.Background(), crawlernodemodel.CrawlerNode{Name: "prod-node"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("Create() ID = %q, want %q", got.ID, want.ID)
	}
}

func TestNodeStore_Create_PropagatesConflict(t *testing.T) {
	store := &mockNodeStore{
		createFn: func(_ context.Context, _ crawlernodemodel.CrawlerNode) (*crawlernodemodel.CrawlerNode, error) {
			return nil, domainerrs.ErrConflict.WithMessage("name already exists")
		},
	}
	_, err := store.Create(context.Background(), crawlernodemodel.CrawlerNode{Name: "dup"})
	if !stderrors.Is(err, domainerrs.ErrConflict) {
		t.Errorf("Create() error = %v, want ErrConflict", err)
	}
}

// --- GetByTokenPrefix ---

func TestNodeStore_GetByTokenPrefix_ReturnsNodes(t *testing.T) {
	prefix := "abcd1234"
	want := []*crawlernodemodel.CrawlerNode{{ID: "uuid-1", TokenPrefix: prefix}}
	store := &mockNodeStore{
		getByPrefixFn: func(_ context.Context, p string) ([]*crawlernodemodel.CrawlerNode, error) {
			if p != prefix {
				t.Errorf("GetByTokenPrefix called with %q, want %q", p, prefix)
			}
			return want, nil
		},
	}
	got, err := store.GetByTokenPrefix(context.Background(), prefix)
	if err != nil {
		t.Fatalf("GetByTokenPrefix() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != want[0].ID {
		t.Errorf("GetByTokenPrefix() = %v, want %v", got, want)
	}
}

func TestNodeStore_GetByTokenPrefix_ReturnsEmptySliceWhenNotFound(t *testing.T) {
	store := &mockNodeStore{
		getByPrefixFn: func(_ context.Context, _ string) ([]*crawlernodemodel.CrawlerNode, error) {
			return nil, nil
		},
	}
	got, err := store.GetByTokenPrefix(context.Background(), "xxxxxxxx")
	if err != nil {
		t.Fatalf("GetByTokenPrefix() unexpected error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetByTokenPrefix() len = %d, want 0", len(got))
	}
}

// GetByTokenPrefix returns all matching rows so the middleware can test each
// token_hash (prefix collision is rare but possible with UUID v4).
func TestNodeStore_GetByTokenPrefix_ReturnsMultipleRowsOnCollision(t *testing.T) {
	prefix := "abcd1234"
	nodes := []*crawlernodemodel.CrawlerNode{
		{ID: "uuid-1", TokenPrefix: prefix},
		{ID: "uuid-2", TokenPrefix: prefix},
	}
	store := &mockNodeStore{
		getByPrefixFn: func(_ context.Context, _ string) ([]*crawlernodemodel.CrawlerNode, error) {
			return nodes, nil
		},
	}
	got, err := store.GetByTokenPrefix(context.Background(), prefix)
	if err != nil {
		t.Fatalf("GetByTokenPrefix() error = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("GetByTokenPrefix() len = %d, want 2 (prefix collision)", len(got))
	}
}

// --- UpdateLastSeen ---

func TestNodeStore_UpdateLastSeen_SuccessOnValidID(t *testing.T) {
	called := false
	store := &mockNodeStore{
		updateLastSeenFn: func(_ context.Context, id string) error {
			called = true
			if id != "uuid-1" {
				t.Errorf("UpdateLastSeen id = %q, want uuid-1", id)
			}
			return nil
		},
	}
	if err := store.UpdateLastSeen(context.Background(), "uuid-1"); err != nil {
		t.Fatalf("UpdateLastSeen() error = %v", err)
	}
	if !called {
		t.Error("UpdateLastSeen: store not called")
	}
}

func TestNodeStore_UpdateLastSeen_ReturnsErrorOnUnknownID(t *testing.T) {
	store := &mockNodeStore{
		updateLastSeenFn: func(_ context.Context, _ string) error {
			return domainerrs.ErrNotFound.WithMessage("node not found")
		},
	}
	err := store.UpdateLastSeen(context.Background(), "unknown-id")
	if !stderrors.Is(err, domainerrs.ErrNotFound) {
		t.Errorf("UpdateLastSeen() error = %v, want ErrNotFound", err)
	}
}

// --- SetStatus ---

func TestNodeStore_SetStatus_Active(t *testing.T) {
	called := false
	store := &mockNodeStore{
		setStatusFn: func(_ context.Context, id, status string) error {
			called = true
			if id != "uuid-1" || status != "active" {
				t.Errorf("SetStatus(%q, %q), want (uuid-1, active)", id, status)
			}
			return nil
		},
	}
	if err := store.SetStatus(context.Background(), "uuid-1", "active"); err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	if !called {
		t.Error("SetStatus: store not called")
	}
}

func TestNodeStore_SetStatus_ReturnsErrorOnNotFound(t *testing.T) {
	store := &mockNodeStore{
		setStatusFn: func(_ context.Context, _, _ string) error {
			return domainerrs.ErrNotFound.WithMessage("node not found")
		},
	}
	err := store.SetStatus(context.Background(), "missing-id", "disabled")
	if !stderrors.Is(err, domainerrs.ErrNotFound) {
		t.Errorf("SetStatus() error = %v, want ErrNotFound", err)
	}
}

// --- List ---

func TestNodeStore_List_ReturnsAllNodesOnEmptyFilter(t *testing.T) {
	now := time.Now()
	want := []crawlernodemodel.CrawlerNode{
		{ID: "uuid-1", Name: "node-1", Status: "active", LastSeenAt: &now},
		{ID: "uuid-2", Name: "node-2", Status: "disabled"},
	}
	store := &mockNodeStore{
		listFn: func(_ context.Context, status string) ([]crawlernodemodel.CrawlerNode, error) {
			if status != "" {
				t.Errorf("List() status = %q, want empty (all)", status)
			}
			return want, nil
		},
	}
	got, err := store.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != len(want) {
		t.Errorf("List() len = %d, want %d", len(got), len(want))
	}
}

func TestNodeStore_List_FiltersOnStatus(t *testing.T) {
	store := &mockNodeStore{
		listFn: func(_ context.Context, status string) ([]crawlernodemodel.CrawlerNode, error) {
			if status != "active" {
				t.Errorf("List() status = %q, want active", status)
			}
			return []crawlernodemodel.CrawlerNode{{ID: "uuid-1", Status: "active"}}, nil
		},
	}
	got, err := store.List(context.Background(), "active")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 {
		t.Errorf("List() len = %d, want 1", len(got))
	}
}

// --- Retire ---

func TestNodeStore_Retire_Success(t *testing.T) {
	called := false
	store := &mockNodeStore{
		retireFn: func(_ context.Context, id string) error {
			called = true
			if id != "uuid-1" {
				t.Errorf("Retire id = %q, want uuid-1", id)
			}
			return nil
		},
	}
	if err := store.Retire(context.Background(), "uuid-1"); err != nil {
		t.Fatalf("Retire() error = %v", err)
	}
	if !called {
		t.Error("Retire: store not called")
	}
}

func TestNodeStore_Retire_ReturnsErrorOnUnknownID(t *testing.T) {
	store := &mockNodeStore{
		retireFn: func(_ context.Context, _ string) error {
			return domainerrs.ErrNotFound.WithMessage("node not found")
		},
	}
	err := store.Retire(context.Background(), "missing-id")
	if !stderrors.Is(err, domainerrs.ErrNotFound) {
		t.Errorf("Retire() error = %v, want ErrNotFound", err)
	}
}

// A second Retire call on an already-retired node returns ErrNotFound (0 rows updated).
// Callers must handle this gracefully (non-fatal idempotent retry).
func TestNodeStore_Retire_NonFatalOnAlreadyRetired(t *testing.T) {
	store := &mockNodeStore{
		retireFn: func(_ context.Context, _ string) error {
			return domainerrs.ErrNotFound.WithMessage("already retired")
		},
	}
	err := store.Retire(context.Background(), "retired-id")
	if err == nil {
		t.Error("Retire() expected non-nil error on already-retired node")
	}
	// The caller (ImplService.RetireNode) must treat ErrNotFound as idempotent.
	if !stderrors.Is(err, domainerrs.ErrNotFound) {
		t.Errorf("Retire() got %v, want ErrNotFound", err)
	}
}
