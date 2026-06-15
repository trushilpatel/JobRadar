package tests

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/ats/core"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	crawlernodecommon "github.com/trushilpatel/jobradar/service/crawlernode/common"
	"github.com/trushilpatel/jobradar/service/crawlernode/dto"
	crawlernodemodel "github.com/trushilpatel/jobradar/service/crawlernode/models"
	"github.com/trushilpatel/jobradar/service/crawlernode/services"
)

// mockStreamPublisher implements StreamPublisher.
type mockStreamPublisher struct {
	publishFn func(ctx context.Context, subject string, payload []byte, msgID string) error
}

func (m *mockStreamPublisher) PublishWithMsgID(ctx context.Context, subject string, payload []byte, msgID string) error {
	return m.publishFn(ctx, subject, payload, msgID)
}

var _ services.StreamPublisher = (*mockStreamPublisher)(nil)

// mockScanSettingsReader implements ScanSettingsReader.
type mockScanSettingsReader struct {
	getAdapterConfigFn func(ctx context.Context, companyID string) (*string, error)
}

func (m *mockScanSettingsReader) GetAdapterConfig(ctx context.Context, companyID string) (*string, error) {
	return m.getAdapterConfigFn(ctx, companyID)
}

var _ services.ScanSettingsReader = (*mockScanSettingsReader)(nil)

// --- helpers ---

func newImpl(
	nodeStore services.NodeStore,
	assignStore services.AssignmentStore,
	ledgerStore services.LedgerStore,
	scanLogStore services.ScanLogStore,
	publisher services.StreamPublisher,
	scanSettings services.ScanSettingsReader,
	tokenSecret string,
) services.NodeService {
	return services.NewImplService(nodeStore, assignStore, ledgerStore, scanLogStore, publisher, scanSettings, tokenSecret)
}

func defaultNodeStore() *mockNodeStore {
	return &mockNodeStore{
		createFn: func(_ context.Context, n crawlernodemodel.CrawlerNode) (*crawlernodemodel.CrawlerNode, error) {
			n.ID = "mock-node-id"
			return &n, nil
		},
		getByPrefixFn:    func(_ context.Context, _ string) ([]*crawlernodemodel.CrawlerNode, error) { return nil, nil },
		updateLastSeenFn: func(_ context.Context, _ string) error { return nil },
		setStatusFn:      func(_ context.Context, _, _ string) error { return nil },
		listFn:           func(_ context.Context, _ string) ([]crawlernodemodel.CrawlerNode, error) { return nil, nil },
		retireFn:         func(_ context.Context, _ string) error { return nil },
	}
}

func defaultAssignStore() *mockAssignmentStore {
	return &mockAssignmentStore{
		claimFn:      func(_ context.Context, _, _ string) (*dto.ScanQueueItem, error) { return nil, nil },
		extendFn:     func(_ context.Context, _ string, _ []string) ([]string, []string, error) { return nil, nil, nil },
		completeFn:   func(_ context.Context, _, _ string) error { return nil },
		failFn:       func(_ context.Context, _, _ string) error { return nil },
		resetStuckFn: func(_ context.Context, _ time.Duration) (int, error) { return 0, nil },
	}
}

func defaultLedgerStore() *mockLedgerStore {
	return &mockLedgerStore{
		hasSeenFn:      func(_ context.Context, _ []string) (map[string]bool, error) { return map[string]bool{}, nil },
		markSeenFn:     func(_ context.Context, _ []string, _, _ string) error { return nil },
		markRemovedFn:  func(_ context.Context, _ []string) error { return nil },
		getActiveIDsFn: func(_ context.Context, _ string) ([]string, error) { return []string{}, nil },
	}
}

func defaultScanLogStore() *mockScanLogStore {
	return &mockScanLogStore{
		recordFn: func(_ context.Context, _ string, _ dto.ScanResult) error { return nil },
	}
}

func defaultPublisher() *mockStreamPublisher {
	return &mockStreamPublisher{
		publishFn: func(_ context.Context, _ string, _ []byte, _ string) error { return nil },
	}
}

func defaultScanSettings() *mockScanSettingsReader {
	return &mockScanSettingsReader{
		getAdapterConfigFn: func(_ context.Context, _ string) (*string, error) { return nil, nil },
	}
}

// --- GetWork ---

func TestImplService_GetWork_ReturnsNilResponseWhenQueueEmpty(t *testing.T) {
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")
	resp, err := svc.GetWork(context.Background(), "node-1", "greenhouse", nil)
	if err != nil {
		t.Fatalf("GetWork() error = %v", err)
	}
	if resp == nil {
		t.Fatal("GetWork() returned nil response (expected WorkResponse with nil assignment for empty queue)")
	}
	if resp.Assignment != nil {
		t.Errorf("GetWork() assignment = %v, want nil (empty queue)", resp.Assignment)
	}
}

func TestImplService_GetWork_ReturnsAssignmentWhenWorkAvailable(t *testing.T) {
	queueItem := &dto.ScanQueueItem{ID: "q-1", CompanyID: "co-1", AdapterType: "greenhouse", Source: "scheduler"}
	assignStore := defaultAssignStore()
	assignStore.claimFn = func(_ context.Context, _, _ string) (*dto.ScanQueueItem, error) {
		return queueItem, nil
	}
	svc := newImpl(defaultNodeStore(), assignStore, defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	resp, err := svc.GetWork(context.Background(), "node-1", "greenhouse", nil)
	if err != nil {
		t.Fatalf("GetWork() error = %v", err)
	}
	if resp.Assignment == nil {
		t.Fatal("GetWork() assignment is nil, want non-nil")
	}
	if resp.Assignment.AssignmentID != "q-1" {
		t.Errorf("GetWork() assignment ID = %q, want q-1", resp.Assignment.AssignmentID)
	}
	if resp.Assignment.CompanyID != "co-1" {
		t.Errorf("GetWork() company ID = %q, want co-1", resp.Assignment.CompanyID)
	}
}

func TestImplService_GetWork_PropagatesHeartbeatMissed(t *testing.T) {
	assignStore := defaultAssignStore()
	assignStore.extendFn = func(_ context.Context, _ string, _ []string) ([]string, []string, error) {
		return []string{"q-1"}, []string{"q-2"}, nil
	}
	svc := newImpl(defaultNodeStore(), assignStore, defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	resp, err := svc.GetWork(context.Background(), "node-1", "greenhouse", []string{"q-1", "q-2"})
	if err != nil {
		t.Fatalf("GetWork() error = %v", err)
	}
	if len(resp.HeartbeatMissed) != 1 || resp.HeartbeatMissed[0] != "q-2" {
		t.Errorf("GetWork() heartbeat_missed = %v, want [q-2]", resp.HeartbeatMissed)
	}
	if len(resp.HeartbeatAcked) != 1 || resp.HeartbeatAcked[0] != "q-1" {
		t.Errorf("GetWork() heartbeat_acked = %v, want [q-1]", resp.HeartbeatAcked)
	}
}

// GetWork no longer touches the ledger — known-ID dedup moved server-side into
// SubmitResults. Confirm the ledger is never consulted during assignment.
func TestImplService_GetWork_DoesNotConsultLedger(t *testing.T) {
	queueItem := &dto.ScanQueueItem{ID: "q-1", CompanyID: "co-1", AdapterType: "greenhouse"}
	assignStore := defaultAssignStore()
	assignStore.claimFn = func(_ context.Context, _, _ string) (*dto.ScanQueueItem, error) {
		return queueItem, nil
	}
	ledger := defaultLedgerStore()
	ledger.getActiveIDsFn = func(_ context.Context, _ string) ([]string, error) {
		t.Error("GetWork() must not call GetActiveJobIDsByCompany — known-ID dedup is server-side now")
		return nil, nil
	}
	svc := newImpl(defaultNodeStore(), assignStore, ledger, defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	resp, err := svc.GetWork(context.Background(), "node-1", "greenhouse", nil)
	if err != nil {
		t.Fatalf("GetWork() error = %v", err)
	}
	if resp.Assignment == nil {
		t.Fatal("GetWork() assignment is nil, want non-nil")
	}
}

// --- SubmitResults ---

// MarkSeenBatch must be called BEFORE PublishWithMsgID — the ledger-before-publish invariant.
// If MarkSeenBatch returns an error, PublishWithMsgID must NOT be called.
func TestImplService_SubmitResults_LedgerBeforePublishInvariant(t *testing.T) {
	publishCalled := false
	ledger := defaultLedgerStore()
	ledger.markSeenFn = func(_ context.Context, _ []string, _, _ string) error {
		return errSentinel // ledger write fails
	}
	publisher := &mockStreamPublisher{
		publishFn: func(_ context.Context, _ string, _ []byte, _ string) error {
			publishCalled = true
			return nil
		},
	}
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), ledger, defaultScanLogStore(), publisher, defaultScanSettings(), "secret")

	req := dto.ResultsRequest{
		AssignmentID: "q-1",
		CompanyID:    "co-1",
		AdapterType:  "greenhouse",
		Jobs:         []core.JobSummary{{ID: "gh-1", Title: "Eng"}},
		Final:        true,
	}
	_, err := svc.SubmitResults(context.Background(), "node-1", req)
	if err == nil {
		t.Fatal("SubmitResults() expected error from MarkSeenBatch failure, got nil")
	}
	if publishCalled {
		t.Error("SubmitResults() PublishWithMsgID was called despite MarkSeenBatch failure — ledger-before-publish violated")
	}
}

func TestImplService_SubmitResults_FinalFalse_DoesNotComplete(t *testing.T) {
	completeCalled := false
	assignStore := defaultAssignStore()
	assignStore.completeFn = func(_ context.Context, _, _ string) error {
		completeCalled = true
		return nil
	}
	svc := newImpl(defaultNodeStore(), assignStore, defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	req := dto.ResultsRequest{
		AssignmentID: "q-1",
		CompanyID:    "co-1",
		AdapterType:  "greenhouse",
		Jobs:         []core.JobSummary{{ID: "gh-1"}},
		Final:        false, // streaming — NOT final
	}
	_, err := svc.SubmitResults(context.Background(), "node-1", req)
	if err != nil {
		t.Fatalf("SubmitResults(final=false) error = %v", err)
	}
	if completeCalled {
		t.Error("SubmitResults(final=false) called CompleteForNode — must not complete on partial batch")
	}
}

func TestImplService_SubmitResults_FinalTrue_Completes(t *testing.T) {
	completeCalled := false
	assignStore := defaultAssignStore()
	assignStore.completeFn = func(_ context.Context, queueID, nodeID string) error {
		completeCalled = true
		if queueID != "q-1" || nodeID != "node-1" {
			t.Errorf("CompleteForNode(%q, %q)", queueID, nodeID)
		}
		return nil
	}
	svc := newImpl(defaultNodeStore(), assignStore, defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	req := dto.ResultsRequest{
		AssignmentID: "q-1",
		CompanyID:    "co-1",
		AdapterType:  "greenhouse",
		Jobs:         []core.JobSummary{{ID: "gh-1"}},
		Final:        true,
	}
	resp, err := svc.SubmitResults(context.Background(), "node-1", req)
	if err != nil {
		t.Fatalf("SubmitResults(final=true) error = %v", err)
	}
	if !completeCalled {
		t.Error("SubmitResults(final=true) did not call CompleteForNode")
	}
	if !resp.Completed {
		t.Error("SubmitResults(final=true) resp.Completed = false, want true")
	}
}

// Removed-job detection is a SERVER-SIDE diff on a final submission:
// previousActive (from the ledger) ∖ submittedIDs = removed.
func TestImplService_SubmitResults_RemovedJobDetection(t *testing.T) {
	var removedIDs []string
	ledger := defaultLedgerStore()
	ledger.getActiveIDsFn = func(_ context.Context, companyID string) ([]string, error) {
		if companyID != "co-1" {
			t.Errorf("GetActiveJobIDsByCompany companyID = %q, want co-1", companyID)
		}
		return []string{"gh-1", "gh-2"}, nil // gh-2 was previously active
	}
	ledger.markRemovedFn = func(_ context.Context, ids []string) error {
		removedIDs = ids
		return nil
	}
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), ledger, defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	req := dto.ResultsRequest{
		AssignmentID: "q-1",
		CompanyID:    "co-1",
		AdapterType:  "greenhouse",
		Jobs:         []core.JobSummary{{ID: "gh-1"}}, // gh-2 is gone from this submission
		Final:        true,
	}
	resp, err := svc.SubmitResults(context.Background(), "node-1", req)
	if err != nil {
		t.Fatalf("SubmitResults() error = %v", err)
	}
	if len(removedIDs) != 1 || removedIDs[0] != "gh-2" {
		t.Errorf("SubmitResults() removed = %v, want [gh-2]", removedIDs)
	}
	if resp.Removed != 1 {
		t.Errorf("SubmitResults() resp.Removed = %d, want 1", resp.Removed)
	}
}

// Removed-job detection must NOT run on a non-final (streaming) submission —
// a partial view would falsely drop jobs still arriving in later batches.
func TestImplService_SubmitResults_RemovedDiffSkippedWhenNotFinal(t *testing.T) {
	activeCalled := false
	markRemovedCalled := false
	ledger := defaultLedgerStore()
	ledger.getActiveIDsFn = func(_ context.Context, _ string) ([]string, error) {
		activeCalled = true
		return []string{"gh-1", "gh-2"}, nil
	}
	ledger.markRemovedFn = func(_ context.Context, _ []string) error {
		markRemovedCalled = true
		return nil
	}
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), ledger, defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	req := dto.ResultsRequest{
		AssignmentID: "q-1",
		CompanyID:    "co-1",
		AdapterType:  "greenhouse",
		Jobs:         []core.JobSummary{{ID: "gh-1"}},
		Final:        false, // partial batch — removed-diff must not run
	}
	resp, err := svc.SubmitResults(context.Background(), "node-1", req)
	if err != nil {
		t.Fatalf("SubmitResults(non-final) error = %v", err)
	}
	if activeCalled {
		t.Error("SubmitResults(non-final): GetActiveJobIDsByCompany must not be called for removed-diff")
	}
	if markRemovedCalled {
		t.Error("SubmitResults(non-final): MarkRemovedBatch must not be called on a partial batch")
	}
	if resp.Removed != 0 {
		t.Errorf("SubmitResults(non-final) resp.Removed = %d, want 0", resp.Removed)
	}
}

// A failing GetActiveJobIDsByCompany during the removed-diff is non-fatal.
func TestImplService_SubmitResults_RemovedDiffActiveErrorNonFatal(t *testing.T) {
	ledger := defaultLedgerStore()
	ledger.getActiveIDsFn = func(_ context.Context, _ string) ([]string, error) {
		return nil, domainerrs.ErrInternal.WithMessage("ledger down")
	}
	ledger.markRemovedFn = func(_ context.Context, _ []string) error {
		t.Error("MarkRemovedBatch must not be called when the active set could not be loaded")
		return nil
	}
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), ledger, defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	resp, err := svc.SubmitResults(context.Background(), "node-1", dto.ResultsRequest{
		AssignmentID: "q-1",
		CompanyID:    "co-1",
		AdapterType:  "greenhouse",
		Jobs:         []core.JobSummary{{ID: "gh-1"}},
		Final:        true,
	})
	if err != nil {
		t.Fatalf("SubmitResults(active error) error = %v, want nil (non-fatal)", err)
	}
	if resp.Removed != 0 {
		t.Errorf("SubmitResults(active error) resp.Removed = %d, want 0", resp.Removed)
	}
}

// MarkRemovedBatch failure during the removed-diff is non-fatal (best-effort).
func TestImplService_SubmitResults_RemovedDiffMarkErrorNonFatal(t *testing.T) {
	ledger := defaultLedgerStore()
	ledger.getActiveIDsFn = func(_ context.Context, _ string) ([]string, error) {
		return []string{"gh-1", "gh-2"}, nil
	}
	ledger.markRemovedFn = func(_ context.Context, _ []string) error {
		return domainerrs.ErrInternal.WithMessage("mark removed failed")
	}
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), ledger, defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	resp, err := svc.SubmitResults(context.Background(), "node-1", dto.ResultsRequest{
		AssignmentID: "q-1",
		CompanyID:    "co-1",
		AdapterType:  "greenhouse",
		Jobs:         []core.JobSummary{{ID: "gh-1"}},
		Final:        true,
	})
	if err != nil {
		t.Fatalf("SubmitResults(mark removed error) error = %v, want nil (non-fatal)", err)
	}
	// resp.Removed is still set even though the ledger write failed (best-effort count).
	if resp.Removed != 1 {
		t.Errorf("SubmitResults(mark removed error) resp.Removed = %d, want 1", resp.Removed)
	}
}

// Non-USA gate: jobs with LocationKnown=true and all locations non-USA must be dropped.
func TestImplService_SubmitResults_NonUSAGate_DropsNonUSAJobs(t *testing.T) {
	publishedIDs := make([]string, 0)
	publisher := &mockStreamPublisher{
		publishFn: func(_ context.Context, _ string, payload []byte, msgID string) error {
			publishedIDs = append(publishedIDs, msgID)
			return nil
		},
	}
	nonUSASeenIDs := make([]string, 0)
	ledger := defaultLedgerStore()
	ledger.markSeenFn = func(_ context.Context, ids []string, _, reason string) error {
		if reason == crawlernodecommon.ScanReasonNonUSA {
			nonUSASeenIDs = append(nonUSASeenIDs, ids...)
		}
		return nil
	}
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), ledger, defaultScanLogStore(), publisher, defaultScanSettings(), "secret")

	req := dto.ResultsRequest{
		AssignmentID: "q-1",
		CompanyID:    "co-1",
		AdapterType:  "greenhouse",
		Jobs: []core.JobSummary{
			{ID: "gh-1", LocationKnown: true, AllLocations: []string{"US"}},     // keep
			{ID: "gh-2", LocationKnown: true, AllLocations: []string{"India"}},  // drop
			{ID: "gh-3", LocationKnown: false, AllLocations: []string{"India"}}, // keep (unknown)
		},
		Final: true,
	}
	_, err := svc.SubmitResults(context.Background(), "node-1", req)
	if err != nil {
		t.Fatalf("SubmitResults() error = %v", err)
	}
	// gh-2 must be in non_usa ledger, not published
	if len(nonUSASeenIDs) != 1 || nonUSASeenIDs[0] != "gh-2" {
		t.Errorf("NonUSA gate: marked %v, want [gh-2]", nonUSASeenIDs)
	}
	// gh-1 and gh-3 must be published
	for _, wantID := range []string{"gh-1", "gh-3"} {
		if !slices.Contains(publishedIDs, wantID) {
			t.Errorf("Expected job %q to be published, publishedIDs = %v", wantID, publishedIDs)
		}
	}
	if slices.Contains(publishedIDs, "gh-2") {
		t.Error("Non-USA job gh-2 must NOT be published, but was")
	}
}

// CompleteForNode returning ErrQueueRowMissing on final:true must be treated as non-fatal (idempotent retry).
func TestImplService_SubmitResults_FinalTrue_IdempotentOnErrQueueRowMissing(t *testing.T) {
	assignStore := defaultAssignStore()
	assignStore.completeFn = func(_ context.Context, _, _ string) error {
		return domainerrs.ErrQueueRowMissing.WithMessage("already completed")
	}
	svc := newImpl(defaultNodeStore(), assignStore, defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	req := dto.ResultsRequest{
		AssignmentID: "q-1",
		CompanyID:    "co-1",
		AdapterType:  "greenhouse",
		Jobs:         []core.JobSummary{{ID: "gh-1"}},
		Final:        true,
	}
	// Should not fail — second call is idempotent
	_, err := svc.SubmitResults(context.Background(), "node-1", req)
	if err != nil {
		t.Fatalf("SubmitResults(idempotent final) error = %v, want nil", err)
	}
}

// Empty jobs with final:true must still complete the assignment and record scan result.
func TestImplService_SubmitResults_FinalTrue_EmptyJobs(t *testing.T) {
	completeCalled := false
	scanLogCalled := false
	assignStore := defaultAssignStore()
	assignStore.completeFn = func(_ context.Context, _, _ string) error {
		completeCalled = true
		return nil
	}
	scanLog := defaultScanLogStore()
	scanLog.recordFn = func(_ context.Context, _ string, r dto.ScanResult) error {
		scanLogCalled = true
		if r.FetchedCount != 0 {
			t.Errorf("RecordScanResult FetchedCount = %d, want 0", r.FetchedCount)
		}
		return nil
	}
	svc := newImpl(defaultNodeStore(), assignStore, defaultLedgerStore(), scanLog, defaultPublisher(), defaultScanSettings(), "secret")

	_, err := svc.SubmitResults(context.Background(), "node-1", dto.ResultsRequest{
		AssignmentID: "q-1",
		CompanyID:    "co-1",
		AdapterType:  "greenhouse",
		Final:        true,
	})
	if err != nil {
		t.Fatalf("SubmitResults(empty final) error = %v", err)
	}
	if !completeCalled {
		t.Error("SubmitResults(empty final) did not call CompleteForNode")
	}
	if !scanLogCalled {
		t.Error("SubmitResults(empty final) did not call RecordScanResult")
	}
}

// --- ReportError ---

func TestImplService_ReportError_CallsFailForNodeAndScanLog(t *testing.T) {
	failCalled := false
	scanLogCalled := false

	assignStore := defaultAssignStore()
	assignStore.failFn = func(_ context.Context, queueID, nodeID string) error {
		failCalled = true
		if queueID != "q-1" || nodeID != "node-1" {
			t.Errorf("FailForNode(%q, %q)", queueID, nodeID)
		}
		return nil
	}
	scanLog := defaultScanLogStore()
	scanLog.recordFn = func(_ context.Context, _ string, r dto.ScanResult) error {
		scanLogCalled = true
		if r.ErrMsg == "" {
			t.Error("RecordScanResult: ErrMsg must be non-empty on error path")
		}
		return nil
	}
	svc := newImpl(defaultNodeStore(), assignStore, defaultLedgerStore(), scanLog, defaultPublisher(), defaultScanSettings(), "secret")

	err := svc.ReportError(context.Background(), "node-1", dto.ErrorRequest{
		AssignmentID: "q-1",
		CompanyID:    "co-1",
		AdapterType:  "greenhouse",
		ErrorMessage: "adapter timeout",
	})
	if err != nil {
		t.Fatalf("ReportError() error = %v", err)
	}
	if !failCalled {
		t.Error("ReportError() did not call FailForNode")
	}
	if !scanLogCalled {
		t.Error("ReportError() did not call RecordScanResult")
	}
}

// ErrQueueRowMissing from FailForNode means row was already reset — non-fatal.
func TestImplService_ReportError_NonFatalOnErrQueueRowMissing(t *testing.T) {
	assignStore := defaultAssignStore()
	assignStore.failFn = func(_ context.Context, _, _ string) error {
		return domainerrs.ErrQueueRowMissing.WithMessage("row already reset")
	}
	svc := newImpl(defaultNodeStore(), assignStore, defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	err := svc.ReportError(context.Background(), "node-1", dto.ErrorRequest{
		AssignmentID: "q-1",
		CompanyID:    "co-1",
		AdapterType:  "greenhouse",
		ErrorMessage: "timeout",
	})
	if err != nil {
		t.Fatalf("ReportError(ErrQueueRowMissing) error = %v, want nil", err)
	}
}

// RecordScanResult is called on the error path even when FailForNode returns ErrQueueRowMissing.
func TestImplService_ReportError_ScanLogCalledEvenOnErrQueueRowMissing(t *testing.T) {
	scanLogCalled := false
	assignStore := defaultAssignStore()
	assignStore.failFn = func(_ context.Context, _, _ string) error {
		return domainerrs.ErrQueueRowMissing.WithMessage("already reset")
	}
	scanLog := defaultScanLogStore()
	scanLog.recordFn = func(_ context.Context, _ string, _ dto.ScanResult) error {
		scanLogCalled = true
		return nil
	}
	svc := newImpl(defaultNodeStore(), assignStore, defaultLedgerStore(), scanLog, defaultPublisher(), defaultScanSettings(), "secret")

	_ = svc.ReportError(context.Background(), "node-1", dto.ErrorRequest{
		AssignmentID: "q-1",
		CompanyID:    "co-1",
		ErrorMessage: "timeout",
	})
	if !scanLogCalled {
		t.Error("ReportError: RecordScanResult must be called even when FailForNode returns ErrQueueRowMissing")
	}
}

// --- ProvisionNode ---

func TestImplService_ProvisionNode_ReturnsTokenOnce(t *testing.T) {
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "test-secret")
	resp, err := svc.ProvisionNode(context.Background(), "prod-node")
	if err != nil {
		t.Fatalf("ProvisionNode() error = %v", err)
	}
	if resp == nil {
		t.Fatal("ProvisionNode() returned nil response")
	}
	if resp.Token == "" {
		t.Error("ProvisionNode() token is empty — must return plaintext token once")
	}
	if resp.ID == "" {
		t.Error("ProvisionNode() node ID is empty")
	}
}

// Two calls produce different tokens — crypto/rand, not math/rand.
func TestImplService_ProvisionNode_TokensAreUnique(t *testing.T) {
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "test-secret")
	r1, err1 := svc.ProvisionNode(context.Background(), "node-a")
	r2, err2 := svc.ProvisionNode(context.Background(), "node-b")
	if err1 != nil || err2 != nil {
		t.Fatalf("ProvisionNode errors: %v / %v", err1, err2)
	}
	if r1.Token == r2.Token {
		t.Error("ProvisionNode() produced duplicate tokens — must use crypto/rand")
	}
}

// --- SetNodeStatus ---

// SetNodeStatus must block transitions from 'retired' (terminal state).
// The DB enforces this via WHERE status != 'retired'; 0 rows affected → ErrNotFound.
func TestImplService_SetNodeStatus_BlocksRetiredToActive(t *testing.T) {
	nodeStore := defaultNodeStore()
	// Simulate DB returning ErrNotFound because WHERE status != 'retired' matches 0 rows.
	nodeStore.setStatusFn = func(_ context.Context, _, _ string) error {
		return domainerrs.ErrNotFound.WithMessage("node is retired or does not exist")
	}
	svc := newImpl(nodeStore, defaultAssignStore(), defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	err := svc.SetNodeStatus(context.Background(), "node-1", "active")
	if err == nil {
		t.Fatal("SetNodeStatus(retired→active) expected error, got nil")
	}
}

// SetNodeStatus same-status no-op should not error.
func TestImplService_SetNodeStatus_SameStatusIsIdempotent(t *testing.T) {
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")
	// defaultNodeStore.setStatusFn returns nil — represents success even for same status.
	err := svc.SetNodeStatus(context.Background(), "node-1", "active")
	if err != nil {
		t.Fatalf("SetNodeStatus(active→active) unexpected error = %v", err)
	}
}

// --- RetireNode ---

func TestImplService_RetireNode_CallsRetire(t *testing.T) {
	retireCalled := false
	nodeStore := defaultNodeStore()
	nodeStore.retireFn = func(_ context.Context, nodeID string) error {
		retireCalled = true
		if nodeID != "node-1" {
			t.Errorf("Retire nodeID = %q, want node-1", nodeID)
		}
		return nil
	}
	svc := newImpl(nodeStore, defaultAssignStore(), defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	if err := svc.RetireNode(context.Background(), "node-1"); err != nil {
		t.Fatalf("RetireNode() error = %v", err)
	}
	if !retireCalled {
		t.Error("RetireNode() did not call NodeStore.Retire")
	}
}

// RetireNode on already-retired node: ErrNotFound from store is treated as non-fatal.
func TestImplService_RetireNode_IdempotentOnAlreadyRetired(t *testing.T) {
	nodeStore := defaultNodeStore()
	nodeStore.retireFn = func(_ context.Context, _ string) error {
		return domainerrs.ErrNotFound.WithMessage("already retired")
	}
	svc := newImpl(nodeStore, defaultAssignStore(), defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	err := svc.RetireNode(context.Background(), "node-1")
	// Should surface as ErrNotFound — callers (REST) map to 404.
	if !stderrors.Is(err, domainerrs.ErrNotFound) {
		t.Errorf("RetireNode(already-retired) error = %v, want ErrNotFound", err)
	}
}

// --- ListNodes ---

func TestImplService_ListNodes_ReturnsViews(t *testing.T) {
	now := time.Now()
	nodeStore := defaultNodeStore()
	nodeStore.listFn = func(_ context.Context, status string) ([]crawlernodemodel.CrawlerNode, error) {
		if status != "" {
			t.Errorf("ListNodes() passed status = %q, want empty", status)
		}
		return []crawlernodemodel.CrawlerNode{
			{ID: "uuid-1", Name: "node-1", Status: "active", LastSeenAt: &now},
			{ID: "uuid-2", Name: "node-2", Status: "disabled"},
		}, nil
	}
	svc := newImpl(nodeStore, defaultAssignStore(), defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	views, err := svc.ListNodes(context.Background(), "")
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("ListNodes() len = %d, want 2", len(views))
	}
	// node-1 was seen recently — must be online.
	if !views[0].Online {
		t.Error("ListNodes() node-1: Online = false, want true (seen just now)")
	}
	// node-2 has no LastSeenAt — must be offline.
	if views[1].Online {
		t.Error("ListNodes() node-2: Online = true, want false (never seen)")
	}
}

func TestImplService_ListNodes_PropagatesStoreError(t *testing.T) {
	nodeStore := defaultNodeStore()
	nodeStore.listFn = func(_ context.Context, _ string) ([]crawlernodemodel.CrawlerNode, error) {
		return nil, domainerrs.ErrInternal.WithMessage("db down")
	}
	svc := newImpl(nodeStore, defaultAssignStore(), defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	_, err := svc.ListNodes(context.Background(), "")
	if err == nil {
		t.Fatal("ListNodes() expected error, got nil")
	}
}

func TestImplService_ListNodes_FiltersByStatus(t *testing.T) {
	nodeStore := defaultNodeStore()
	nodeStore.listFn = func(_ context.Context, status string) ([]crawlernodemodel.CrawlerNode, error) {
		if status != "active" {
			t.Errorf("ListNodes(active) status = %q, want active", status)
		}
		return []crawlernodemodel.CrawlerNode{{ID: "uuid-1", Name: "node-1", Status: "active"}}, nil
	}
	svc := newImpl(nodeStore, defaultAssignStore(), defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	views, err := svc.ListNodes(context.Background(), "active")
	if err != nil {
		t.Fatalf("ListNodes(active) error = %v", err)
	}
	if len(views) != 1 || views[0].Status != "active" {
		t.Errorf("ListNodes(active) = %v", views)
	}
}

// --- BaseNodeService ---

// ProvisionNode: nodeStore.Create failure must propagate.
func TestImplService_ProvisionNode_PropagatesStoreError(t *testing.T) {
	nodeStore := defaultNodeStore()
	nodeStore.createFn = func(_ context.Context, _ crawlernodemodel.CrawlerNode) (*crawlernodemodel.CrawlerNode, error) {
		return nil, domainerrs.ErrConflict.WithMessage("name taken")
	}
	svc := newImpl(nodeStore, defaultAssignStore(), defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")
	_, err := svc.ProvisionNode(context.Background(), "dup-node")
	if err == nil {
		t.Fatal("ProvisionNode() expected error from store, got nil")
	}
}

// ReportError: non-fatal scan-log error must not surface to caller.
func TestImplService_ReportError_NonFatalScanLogError(t *testing.T) {
	scanLog := defaultScanLogStore()
	scanLog.recordFn = func(_ context.Context, _ string, _ dto.ScanResult) error {
		return domainerrs.ErrInternal.WithMessage("log write failed")
	}
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), defaultLedgerStore(), scanLog, defaultPublisher(), defaultScanSettings(), "secret")
	err := svc.ReportError(context.Background(), "node-1", dto.ErrorRequest{AssignmentID: "q-1"})
	if err != nil {
		t.Fatalf("ReportError() scan log error should be non-fatal, got %v", err)
	}
}

// GetWork: ExtendHeartbeats error must propagate.
func TestImplService_GetWork_PropagatesExtendHeartbeatsError(t *testing.T) {
	assignStore := defaultAssignStore()
	assignStore.extendFn = func(_ context.Context, _ string, _ []string) ([]string, []string, error) {
		return nil, nil, domainerrs.ErrInternal.WithMessage("extend failed")
	}
	svc := newImpl(defaultNodeStore(), assignStore, defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")
	_, err := svc.GetWork(context.Background(), "node-1", "greenhouse", []string{"q-1"})
	if err == nil {
		t.Fatal("GetWork() expected ExtendHeartbeats error, got nil")
	}
}

// GetWork: ClaimNextForNode error must propagate.
func TestImplService_GetWork_PropagatesClaimNextError(t *testing.T) {
	assignStore := defaultAssignStore()
	assignStore.claimFn = func(_ context.Context, _, _ string) (*dto.ScanQueueItem, error) {
		return nil, domainerrs.ErrInternal.WithMessage("claim failed")
	}
	svc := newImpl(defaultNodeStore(), assignStore, defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")
	_, err := svc.GetWork(context.Background(), "node-1", "greenhouse", nil)
	if err == nil {
		t.Fatal("GetWork() expected ClaimNextForNode error, got nil")
	}
}

// SubmitResults: HasSeenBatch error must propagate.
func TestImplService_SubmitResults_PropagatesHasSeenBatchError(t *testing.T) {
	ledger := defaultLedgerStore()
	ledger.hasSeenFn = func(_ context.Context, _ []string) (map[string]bool, error) {
		return nil, domainerrs.ErrInternal.WithMessage("ledger error")
	}
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), ledger, defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")
	_, err := svc.SubmitResults(context.Background(), "node-1", dto.ResultsRequest{
		CompanyID:   "co-1",
		AdapterType: "greenhouse",
		Jobs:        []core.JobSummary{{ID: "gh-1", LocationKnown: false}},
	})
	if err == nil {
		t.Fatal("SubmitResults() expected HasSeenBatch error, got nil")
	}
}

// SubmitResults: MarkSeenBatch (non_usa) error must propagate.
func TestImplService_SubmitResults_PropagatesNonUSAMarkError(t *testing.T) {
	ledger := defaultLedgerStore()
	ledger.markSeenFn = func(_ context.Context, _ []string, _, reason string) error {
		if reason == crawlernodecommon.ScanReasonNonUSA {
			return domainerrs.ErrInternal.WithMessage("ledger error")
		}
		return nil
	}
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), ledger, defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")
	_, err := svc.SubmitResults(context.Background(), "node-1", dto.ResultsRequest{
		CompanyID:   "co-1",
		AdapterType: "greenhouse",
		// LocationKnown=true + non-USA location → goes to non-USA mark path
		Jobs: []core.JobSummary{{
			ID: "gh-1", LocationKnown: true,
			AllLocations: []string{"India"},
		}},
	})
	if err == nil {
		t.Fatal("SubmitResults() expected non-USA MarkSeenBatch error, got nil")
	}
}

// SetNodeStatus with status="retired" must delegate to RetireNode.
func TestImplService_SetNodeStatus_ToRetiredDelegatesRetireNode(t *testing.T) {
	retireCalled := false
	nodeStore := defaultNodeStore()
	nodeStore.retireFn = func(_ context.Context, _ string) error {
		retireCalled = true
		return nil
	}
	svc := newImpl(nodeStore, defaultAssignStore(), defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	if err := svc.SetNodeStatus(context.Background(), "node-1", "retired"); err != nil {
		t.Fatalf("SetNodeStatus(→retired) error = %v", err)
	}
	if !retireCalled {
		t.Error("SetNodeStatus(→retired): RetireNode not called")
	}
}

// ReportError with a fatal (non-ErrQueueRowMissing) FailForNode error must return it.
func TestImplService_ReportError_ReturnsFatalFailError(t *testing.T) {
	fatalErr := domainerrs.ErrInternal.WithMessage("db down")
	assignStore := defaultAssignStore()
	assignStore.failFn = func(_ context.Context, _, _ string) error { return fatalErr }
	svc := newImpl(defaultNodeStore(), assignStore, defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), defaultScanSettings(), "secret")

	err := svc.ReportError(context.Background(), "node-1", dto.ErrorRequest{AssignmentID: "q-1"})
	if err == nil {
		t.Fatal("ReportError() expected fatal error, got nil")
	}
}

// SubmitResults with JobContent set publishes a payload whose job_content key is
// present, with CompanyID/Adapter stamped and the subject jobs.raw.<adapter>.
func TestImplService_SubmitResults_WithJobContent_PublishesStampedPayload(t *testing.T) {
	var gotSubject string
	var gotPayload []byte
	var gotMsgID string
	publisher := &mockStreamPublisher{
		publishFn: func(_ context.Context, subject string, payload []byte, msgID string) error {
			gotSubject = subject
			gotPayload = payload
			gotMsgID = msgID
			return nil
		},
	}
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), defaultLedgerStore(), defaultScanLogStore(), publisher, defaultScanSettings(), "secret")
	req := dto.ResultsRequest{
		AssignmentID: "q-1",
		CompanyID:    "co-1",
		AdapterType:  "greenhouse",
		Jobs: []core.JobSummary{{
			// CompanyID/Adapter deliberately empty — v2 mappers leave them blank;
			// the orchestrator must stamp them before publishing.
			ID:    "gh-1",
			Title: "SWE",
			JobContent: &core.JobContent{
				Description:  "Build things",
				Requirements: "5yr exp",
				Compensation: "200k",
				Department:   "Engineering",
			},
		}},
		Final: false,
	}
	resp, err := svc.SubmitResults(context.Background(), "node-1", req)
	if err != nil {
		t.Fatalf("SubmitResults(JobContent) error = %v", err)
	}
	if gotPayload == nil {
		t.Fatal("SubmitResults(JobContent): publish not called")
	}
	if resp.New != 1 {
		t.Errorf("SubmitResults(JobContent) New = %d, want 1", resp.New)
	}
	if gotSubject != "jobs.raw.greenhouse" {
		t.Errorf("subject = %q, want jobs.raw.greenhouse", gotSubject)
	}
	if gotMsgID != "gh-1" {
		t.Errorf("msgID = %q, want gh-1", gotMsgID)
	}

	var published core.JobSummary
	if err := json.Unmarshal(gotPayload, &published); err != nil {
		t.Fatalf("payload not valid core.JobSummary JSON: %v", err)
	}
	if published.CompanyID != "co-1" {
		t.Errorf("published company_id = %q, want co-1 (stamped)", published.CompanyID)
	}
	if published.Adapter != "greenhouse" {
		t.Errorf("published adapter = %q, want greenhouse (stamped)", published.Adapter)
	}
	if published.JobContent == nil {
		t.Fatal("published job_content is nil, want present")
	}
	if published.JobContent.Compensation != "200k" {
		t.Errorf("published job_content.compensation = %q, want 200k", published.JobContent.Compensation)
	}
	// job_content key must literally appear in the wire JSON when content exists.
	if !strings.Contains(string(gotPayload), `"job_content"`) {
		t.Errorf("payload missing job_content key: %s", gotPayload)
	}
}

// When a job has no JobContent, the omitempty job_content key must be absent.
func TestImplService_SubmitResults_NoJobContent_OmitsKey(t *testing.T) {
	var gotPayload []byte
	publisher := &mockStreamPublisher{
		publishFn: func(_ context.Context, _ string, payload []byte, _ string) error {
			gotPayload = payload
			return nil
		},
	}
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), defaultLedgerStore(), defaultScanLogStore(), publisher, defaultScanSettings(), "secret")
	_, err := svc.SubmitResults(context.Background(), "node-1", dto.ResultsRequest{
		AssignmentID: "q-1",
		CompanyID:    "co-1",
		AdapterType:  "greenhouse",
		Jobs:         []core.JobSummary{{ID: "gh-1", Title: "SWE"}},
		Final:        false,
	})
	if err != nil {
		t.Fatalf("SubmitResults(no content) error = %v", err)
	}
	if strings.Contains(string(gotPayload), `"job_content"`) {
		t.Errorf("payload should omit job_content when absent: %s", gotPayload)
	}
}

// GetWork when GetAdapterConfig fails must drain the queue entry and return empty.
func TestImplService_GetWork_DrainOnAdapterConfigError(t *testing.T) {
	completeCalled := false
	assignStore := defaultAssignStore()
	assignStore.claimFn = func(_ context.Context, _, _ string) (*dto.ScanQueueItem, error) {
		return &dto.ScanQueueItem{ID: "q-1", CompanyID: "co-1", AdapterType: "greenhouse"}, nil
	}
	assignStore.completeFn = func(_ context.Context, _, _ string) error {
		completeCalled = true
		return nil
	}
	scanSettings := &mockScanSettingsReader{
		getAdapterConfigFn: func(_ context.Context, _ string) (*string, error) {
			return nil, domainerrs.ErrNotFound.WithMessage("no settings")
		},
	}
	svc := newImpl(defaultNodeStore(), assignStore, defaultLedgerStore(), defaultScanLogStore(), defaultPublisher(), scanSettings, "secret")

	resp, err := svc.GetWork(context.Background(), "node-1", "greenhouse", nil)
	if err != nil {
		t.Fatalf("GetWork(adapterConfigError) error = %v", err)
	}
	if resp.Assignment != nil {
		t.Error("GetWork(adapterConfigError): Assignment should be nil (drained)")
	}
	if !completeCalled {
		t.Error("GetWork(adapterConfigError): CompleteForNode not called to drain")
	}
}

// isNonUSA with LocationKnown=true but empty AllLocations returns false (no locations to check).
func TestImplService_SubmitResults_NonUSA_EmptyLocations_PassesThrough(t *testing.T) {
	publishCalled := false
	publisher := &mockStreamPublisher{
		publishFn: func(_ context.Context, _ string, _ []byte, _ string) error {
			publishCalled = true
			return nil
		},
	}
	svc := newImpl(defaultNodeStore(), defaultAssignStore(), defaultLedgerStore(), defaultScanLogStore(), publisher, defaultScanSettings(), "secret")
	req := dto.ResultsRequest{
		CompanyID:   "co-1",
		AdapterType: "greenhouse",
		Jobs: []core.JobSummary{{
			ID:            "gh-1",
			LocationKnown: true,
			AllLocations:  []string{}, // empty — not definitively non-USA
		}},
	}
	_, err := svc.SubmitResults(context.Background(), "node-1", req)
	if err != nil {
		t.Fatalf("SubmitResults(emptyLocations) error = %v", err)
	}
	// Job should pass through (not dropped as non-USA) and be published.
	if !publishCalled {
		t.Error("SubmitResults(emptyLocations): job was incorrectly dropped as non-USA")
	}
}

// --- BaseNodeService ---

// NewBaseNodeService panics on nil next to surface wiring mistakes early.
func TestBaseNodeService_PanicsOnNilNext(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewBaseNodeService(nil) did not panic")
		}
	}()
	services.NewBaseNodeService(nil)
}
