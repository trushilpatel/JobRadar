package tests

import (
	"context"
	stderrors "errors"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/crawlernode/dto"
	crawlernodemodel "github.com/trushilpatel/jobradar/service/crawlernode/models"
	"github.com/trushilpatel/jobradar/service/crawlernode/services"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// ─── Mock inner services ────────────────────────────────────────────────────

// mockLedgerInner implements jobsvc.LedgerService for testing ledgerRepositoryService.
type mockLedgerInner struct {
	hasSeenFn      func(ctx context.Context, jobIDs []string) (map[string]bool, error)
	markSeenFn     func(ctx context.Context, jobIDs []string, companyID, reason string) error
	markRemovedFn  func(ctx context.Context, jobIDs []string) error
	getActiveIDsFn func(ctx context.Context, companyID string) ([]string, error)
}

func (m *mockLedgerInner) HasSeenBatch(ctx context.Context, ids []string) (map[string]bool, error) {
	return m.hasSeenFn(ctx, ids)
}
func (m *mockLedgerInner) MarkSeenBatch(ctx context.Context, ids []string, companyID, reason string) error {
	return m.markSeenFn(ctx, ids, companyID, reason)
}
func (m *mockLedgerInner) MarkRemovedBatch(ctx context.Context, ids []string) error {
	return m.markRemovedFn(ctx, ids)
}
func (m *mockLedgerInner) GetActiveJobIDsByCompany(ctx context.Context, companyID string) ([]string, error) {
	return m.getActiveIDsFn(ctx, companyID)
}

var _ jobsvc.LedgerService = (*mockLedgerInner)(nil)

// mockScanLogInner implements scansvc.ScanLogWriter for testing scanLogRepositoryService.
type mockScanLogInner struct {
	insertFn func(ctx context.Context, companyID string, fetched, new, dur int, errMsg *string) error
	recordFn func(ctx context.Context, companyID, status string, fetched, new, dur int, errMsg *string) error
}

func (m *mockScanLogInner) InsertCompanyScanLog(ctx context.Context, companyID string, fetched, new, dur int, errMsg *string) error {
	return m.insertFn(ctx, companyID, fetched, new, dur, errMsg)
}
func (m *mockScanLogInner) RecordScanResult(ctx context.Context, companyID, status string, fetched, new, dur int, errMsg *string) error {
	return m.recordFn(ctx, companyID, status, fetched, new, dur, errMsg)
}

var _ scansvc.ScanLogWriter = (*mockScanLogInner)(nil)

// ─── ledgerRepositoryService tests ─────────────────────────────────────────

func TestLedgerRepositoryService_HasSeenBatch_Delegates(t *testing.T) {
	called := false
	inner := &mockLedgerInner{
		hasSeenFn: func(_ context.Context, ids []string) (map[string]bool, error) {
			called = true
			return map[string]bool{ids[0]: true}, nil
		},
	}
	store := services.NewLedgerRepositoryService(inner)
	got, err := store.HasSeenBatch(context.Background(), []string{"gh-1"})
	if err != nil || !called {
		t.Fatalf("HasSeenBatch delegate failed: err=%v called=%v", err, called)
	}
	if !got["gh-1"] {
		t.Error("HasSeenBatch() gh-1 not in result")
	}
}

func TestLedgerRepositoryService_HasSeenBatch_EmptySliceNoDelegation(t *testing.T) {
	called := false
	inner := &mockLedgerInner{
		hasSeenFn: func(_ context.Context, _ []string) (map[string]bool, error) {
			called = true
			return nil, nil
		},
	}
	store := services.NewLedgerRepositoryService(inner)
	got, err := store.HasSeenBatch(context.Background(), []string{})
	if err != nil {
		t.Fatalf("HasSeenBatch(empty) error = %v", err)
	}
	if called {
		t.Error("HasSeenBatch(empty): inner was called — must short-circuit")
	}
	if len(got) != 0 {
		t.Errorf("HasSeenBatch(empty) len = %d, want 0", len(got))
	}
}

func TestLedgerRepositoryService_HasSeenBatch_PropagatesError(t *testing.T) {
	inner := &mockLedgerInner{
		hasSeenFn: func(_ context.Context, _ []string) (map[string]bool, error) {
			return nil, stderrors.New("db down")
		},
	}
	store := services.NewLedgerRepositoryService(inner)
	_, err := store.HasSeenBatch(context.Background(), []string{"gh-1"})
	if err == nil {
		t.Fatal("HasSeenBatch() expected error from inner, got nil")
	}
}

func TestLedgerRepositoryService_MarkSeenBatch_Delegates(t *testing.T) {
	called := false
	inner := &mockLedgerInner{
		markSeenFn: func(_ context.Context, ids []string, companyID, reason string) error {
			called = true
			if companyID != "co-1" || reason != "fetch" {
				t.Errorf("MarkSeenBatch called with (%q, %q), want (co-1, fetch)", companyID, reason)
			}
			return nil
		},
	}
	store := services.NewLedgerRepositoryService(inner)
	if err := store.MarkSeenBatch(context.Background(), []string{"gh-1"}, "co-1", "fetch"); err != nil {
		t.Fatalf("MarkSeenBatch() error = %v", err)
	}
	if !called {
		t.Error("MarkSeenBatch() inner not called")
	}
}

func TestLedgerRepositoryService_MarkSeenBatch_PropagatesError(t *testing.T) {
	inner := &mockLedgerInner{
		markSeenFn: func(_ context.Context, _ []string, _, _ string) error {
			return stderrors.New("db error")
		},
	}
	store := services.NewLedgerRepositoryService(inner)
	err := store.MarkSeenBatch(context.Background(), []string{"gh-1"}, "co-1", "fetch")
	if err == nil {
		t.Fatal("MarkSeenBatch() expected error from inner, got nil")
	}
}

func TestLedgerRepositoryService_MarkSeenBatch_EmptyNoOp(t *testing.T) {
	called := false
	inner := &mockLedgerInner{
		markSeenFn: func(_ context.Context, _ []string, _, _ string) error {
			called = true
			return nil
		},
	}
	store := services.NewLedgerRepositoryService(inner)
	if err := store.MarkSeenBatch(context.Background(), []string{}, "co-1", "fetch"); err != nil {
		t.Fatalf("MarkSeenBatch(empty) error = %v", err)
	}
	if called {
		t.Error("MarkSeenBatch(empty): inner was called — must short-circuit")
	}
}

func TestLedgerRepositoryService_MarkRemovedBatch_Delegates(t *testing.T) {
	called := false
	inner := &mockLedgerInner{
		markRemovedFn: func(_ context.Context, ids []string) error {
			called = true
			return nil
		},
	}
	store := services.NewLedgerRepositoryService(inner)
	if err := store.MarkRemovedBatch(context.Background(), []string{"gh-1"}); err != nil {
		t.Fatalf("MarkRemovedBatch() error = %v", err)
	}
	if !called {
		t.Error("MarkRemovedBatch() inner not called")
	}
}

func TestLedgerRepositoryService_MarkRemovedBatch_PropagatesError(t *testing.T) {
	inner := &mockLedgerInner{
		markRemovedFn: func(_ context.Context, _ []string) error {
			return stderrors.New("db error")
		},
	}
	store := services.NewLedgerRepositoryService(inner)
	err := store.MarkRemovedBatch(context.Background(), []string{"gh-1"})
	if err == nil {
		t.Fatal("MarkRemovedBatch() expected error from inner, got nil")
	}
}

func TestLedgerRepositoryService_MarkRemovedBatch_EmptyNoOp(t *testing.T) {
	called := false
	inner := &mockLedgerInner{
		markRemovedFn: func(_ context.Context, _ []string) error { called = true; return nil },
	}
	store := services.NewLedgerRepositoryService(inner)
	if err := store.MarkRemovedBatch(context.Background(), []string{}); err != nil {
		t.Fatalf("MarkRemovedBatch(empty) error = %v", err)
	}
	if called {
		t.Error("MarkRemovedBatch(empty): inner called — must short-circuit")
	}
}

func TestLedgerRepositoryService_GetActiveJobIDsByCompany_Delegates(t *testing.T) {
	inner := &mockLedgerInner{
		getActiveIDsFn: func(_ context.Context, companyID string) ([]string, error) {
			if companyID != "co-1" {
				t.Errorf("GetActiveJobIDsByCompany companyID = %q, want co-1", companyID)
			}
			return []string{"gh-1", "gh-2"}, nil
		},
	}
	store := services.NewLedgerRepositoryService(inner)
	ids, err := store.GetActiveJobIDsByCompany(context.Background(), "co-1")
	if err != nil {
		t.Fatalf("GetActiveJobIDsByCompany() error = %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("GetActiveJobIDsByCompany() len = %d, want 2", len(ids))
	}
}

func TestLedgerRepositoryService_GetActiveJobIDsByCompany_PropagatesError(t *testing.T) {
	inner := &mockLedgerInner{
		getActiveIDsFn: func(_ context.Context, _ string) ([]string, error) {
			return nil, stderrors.New("db error")
		},
	}
	store := services.NewLedgerRepositoryService(inner)
	_, err := store.GetActiveJobIDsByCompany(context.Background(), "co-1")
	if err == nil {
		t.Fatal("GetActiveJobIDsByCompany() expected error, got nil")
	}
}

// ─── scanLogRepositoryService tests ────────────────────────────────────────

func TestScanLogRepositoryService_RecordScanResult_Success(t *testing.T) {
	called := false
	inner := &mockScanLogInner{
		insertFn: func(_ context.Context, _ string, _, _, _ int, _ *string) error { return nil },
		recordFn: func(_ context.Context, companyID, status string, fetched, new, dur int, errMsg *string) error {
			called = true
			if companyID != "co-1" || status != "ok" {
				t.Errorf("RecordScanResult called with (%q, %q)", companyID, status)
			}
			if errMsg != nil {
				t.Errorf("RecordScanResult errMsg = %v, want nil", errMsg)
			}
			return nil
		},
	}
	store := services.NewScanLogRepositoryService(inner)
	if err := store.RecordScanResult(context.Background(), "co-1", dto.ScanResult{FetchedCount: 5, NewCount: 2, DurationMs: 100}); err != nil {
		t.Fatalf("RecordScanResult() error = %v", err)
	}
	if !called {
		t.Error("RecordScanResult() inner not called")
	}
}

func TestScanLogRepositoryService_RecordScanResult_ErrorStatus(t *testing.T) {
	inner := &mockScanLogInner{
		insertFn: func(_ context.Context, _ string, _, _, _ int, _ *string) error { return nil },
		recordFn: func(_ context.Context, _, status string, _, _, _ int, errMsg *string) error {
			if status != "error" {
				return stderrors.New("expected status=error")
			}
			if errMsg == nil || *errMsg != "timeout" {
				return stderrors.New("expected errMsg=timeout")
			}
			return nil
		},
	}
	store := services.NewScanLogRepositoryService(inner)
	err := store.RecordScanResult(context.Background(), "co-1", dto.ScanResult{ErrMsg: "timeout"})
	if err != nil {
		t.Fatalf("RecordScanResult(error) error = %v", err)
	}
}

func TestScanLogRepositoryService_RecordScanResult_PropagatesError(t *testing.T) {
	inner := &mockScanLogInner{
		insertFn: func(_ context.Context, _ string, _, _, _ int, _ *string) error { return nil },
		recordFn: func(_ context.Context, _, _ string, _, _, _ int, _ *string) error {
			return stderrors.New("write failed")
		},
	}
	store := services.NewScanLogRepositoryService(inner)
	err := store.RecordScanResult(context.Background(), "co-1", dto.ScanResult{})
	if err == nil {
		t.Fatal("RecordScanResult() expected error, got nil")
	}
}

// ─── BaseNodeService delegation tests ──────────────────────────────────────

// passThruWrapper embeds BaseNodeService without overriding anything.
// Calling its methods exercises all BaseNodeService delegation paths.
type passThruWrapper struct {
	services.BaseNodeService
}

// mockFullNodeService is a complete NodeService mock for testing delegation.
type mockFullNodeService struct {
	getWorkFn       func(ctx context.Context, nodeID, adapterType string, holdingIDs []string) (*dto.WorkResponse, error)
	submitResultsFn func(ctx context.Context, nodeID string, req dto.ResultsRequest) (*dto.ResultsResponse, error)
	reportErrorFn   func(ctx context.Context, nodeID string, req dto.ErrorRequest) error
	provisionFn     func(ctx context.Context, name string) (*dto.CreateNodeResponse, error)
	listNodesFn     func(ctx context.Context, status string) ([]dto.NodeView, error)
	setStatusFn     func(ctx context.Context, nodeID, status string) error
	retireFn        func(ctx context.Context, nodeID string) error
}

func (m *mockFullNodeService) GetWork(ctx context.Context, nodeID, adapterType string, holdingIDs []string) (*dto.WorkResponse, error) {
	return m.getWorkFn(ctx, nodeID, adapterType, holdingIDs)
}
func (m *mockFullNodeService) SubmitResults(ctx context.Context, nodeID string, req dto.ResultsRequest) (*dto.ResultsResponse, error) {
	return m.submitResultsFn(ctx, nodeID, req)
}
func (m *mockFullNodeService) ReportError(ctx context.Context, nodeID string, req dto.ErrorRequest) error {
	return m.reportErrorFn(ctx, nodeID, req)
}
func (m *mockFullNodeService) ProvisionNode(ctx context.Context, name string) (*dto.CreateNodeResponse, error) {
	return m.provisionFn(ctx, name)
}
func (m *mockFullNodeService) ListNodes(ctx context.Context, status string) ([]dto.NodeView, error) {
	return m.listNodesFn(ctx, status)
}
func (m *mockFullNodeService) SetNodeStatus(ctx context.Context, nodeID, status string) error {
	return m.setStatusFn(ctx, nodeID, status)
}
func (m *mockFullNodeService) RetireNode(ctx context.Context, nodeID string) error {
	return m.retireFn(ctx, nodeID)
}

var _ services.NodeService = (*mockFullNodeService)(nil)

func newPassThru(next services.NodeService) *passThruWrapper {
	return &passThruWrapper{BaseNodeService: services.NewBaseNodeService(next)}
}

func TestBaseNodeService_GetWork_Delegates(t *testing.T) {
	called := false
	mock := &mockFullNodeService{
		getWorkFn: func(_ context.Context, _, _ string, _ []string) (*dto.WorkResponse, error) {
			called = true
			return &dto.WorkResponse{}, nil
		},
	}
	svc := newPassThru(mock)
	_, _ = svc.GetWork(context.Background(), "node-1", "greenhouse", nil)
	if !called {
		t.Error("GetWork: BaseNodeService did not delegate to next")
	}
}

func TestBaseNodeService_SubmitResults_Delegates(t *testing.T) {
	called := false
	mock := &mockFullNodeService{
		submitResultsFn: func(_ context.Context, _ string, _ dto.ResultsRequest) (*dto.ResultsResponse, error) {
			called = true
			return &dto.ResultsResponse{}, nil
		},
	}
	svc := newPassThru(mock)
	_, _ = svc.SubmitResults(context.Background(), "node-1", dto.ResultsRequest{})
	if !called {
		t.Error("SubmitResults: BaseNodeService did not delegate to next")
	}
}

func TestBaseNodeService_ReportError_Delegates(t *testing.T) {
	called := false
	mock := &mockFullNodeService{
		reportErrorFn: func(_ context.Context, _ string, _ dto.ErrorRequest) error {
			called = true
			return nil
		},
	}
	svc := newPassThru(mock)
	_ = svc.ReportError(context.Background(), "node-1", dto.ErrorRequest{})
	if !called {
		t.Error("ReportError: BaseNodeService did not delegate to next")
	}
}

func TestBaseNodeService_ProvisionNode_Delegates(t *testing.T) {
	called := false
	mock := &mockFullNodeService{
		provisionFn: func(_ context.Context, _ string) (*dto.CreateNodeResponse, error) {
			called = true
			return &dto.CreateNodeResponse{NodeView: dto.NodeView{ID: "n1"}, Token: "tok"}, nil
		},
	}
	svc := newPassThru(mock)
	resp, err := svc.ProvisionNode(context.Background(), "my-node")
	if err != nil || !called {
		t.Fatalf("ProvisionNode: err=%v called=%v", err, called)
	}
	if resp.ID != "n1" {
		t.Errorf("ProvisionNode() ID = %q, want n1", resp.ID)
	}
}

func TestBaseNodeService_ListNodes_Delegates(t *testing.T) {
	called := false
	mock := &mockFullNodeService{
		listNodesFn: func(_ context.Context, _ string) ([]dto.NodeView, error) {
			called = true
			return []dto.NodeView{{ID: "n1"}}, nil
		},
	}
	svc := newPassThru(mock)
	views, err := svc.ListNodes(context.Background(), "")
	if err != nil || !called {
		t.Fatalf("ListNodes: err=%v called=%v", err, called)
	}
	if len(views) != 1 {
		t.Errorf("ListNodes() len = %d, want 1", len(views))
	}
}

func TestBaseNodeService_SetNodeStatus_Delegates(t *testing.T) {
	called := false
	mock := &mockFullNodeService{
		setStatusFn: func(_ context.Context, _, _ string) error {
			called = true
			return nil
		},
	}
	svc := newPassThru(mock)
	_ = svc.SetNodeStatus(context.Background(), "node-1", "disabled")
	if !called {
		t.Error("SetNodeStatus: BaseNodeService did not delegate to next")
	}
}

func TestBaseNodeService_RetireNode_Delegates(t *testing.T) {
	called := false
	mock := &mockFullNodeService{
		retireFn: func(_ context.Context, _ string) error {
			called = true
			return domainerrs.ErrNotFound.WithMessage("already retired")
		},
	}
	svc := newPassThru(mock)
	err := svc.RetireNode(context.Background(), "node-1")
	if !stderrors.Is(err, domainerrs.ErrNotFound) {
		t.Errorf("RetireNode: error = %v, want ErrNotFound", err)
	}
	if !called {
		t.Error("RetireNode: BaseNodeService did not delegate to next")
	}
}

// ─── NewNodeRepositoryService compile check ─────────────────────────────────

// Compile-time check: NewNodeRepositoryService returns NodeStore.
var _ services.NodeStore = services.NewNodeRepositoryService(nil)

// ─── NewLedgerRepositoryService / NewScanLogRepositoryService compile checks ──

// Compile-time check: constructors return correct interfaces.
var _ services.LedgerStore = services.NewLedgerRepositoryService(nil)
var _ services.ScanLogStore = services.NewScanLogRepositoryService(nil)

// CrawlerNode model field check.
var _ = crawlernodemodel.CrawlerNode{}
