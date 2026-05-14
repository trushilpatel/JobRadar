package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// countingTechTagWriter records calls to InsertTechTags so tests can assert
// whether the impl fires this best-effort write.
type countingTechTagWriter struct {
	calls     int
	lastTechs []string
}

func (c *countingTechTagWriter) InsertTechTags(_ context.Context, _, _ string, techs []string, _ time.Time) error {
	c.calls++
	c.lastTechs = techs
	return nil
}

var _ jobsvc.TechTagWriter = (*countingTechTagWriter)(nil)

// TestMarkLedgerIrrelevant_HappyPath asserts the irrelevant pathway writes
// SeenReasonLLMIrrelevant. This routes a job out of the active filtering
// pipeline without dropping it from the ledger.
func TestMarkLedgerIrrelevant_HappyPath(t *testing.T) {
	jobs := &pipelineJobWriter{}
	ledger := &pipelineLedgerStore{}
	snapshot := &pipelineSnapshotWriter{}

	svc := jobsvc.NewPipelineService(jobs, ledger, snapshot, &pipelineTechTagWriter{})
	if err := svc.MarkLedgerIrrelevant(context.Background(), "job-xyz"); err != nil {
		t.Fatalf("MarkLedgerIrrelevant err = %v", err)
	}
	if ledger.updateReasonJobID != "job-xyz" {
		t.Errorf("ledger job ID = %q, want %q", ledger.updateReasonJobID, "job-xyz")
	}
	if ledger.updateReasonVal != jobcommon.SeenReasonLLMIrrelevant {
		t.Errorf("ledger reason = %q, want %q", ledger.updateReasonVal, jobcommon.SeenReasonLLMIrrelevant)
	}
}

// TestMarkLedgerIrrelevant_PropagatesLedgerError — when the ledger update
// fails, the error bubbles up so the worker can route the job to retry.
func TestMarkLedgerIrrelevant_PropagatesLedgerError(t *testing.T) {
	ledger := &pipelineLedgerStore{updateReasonErr: errors.New("ledger down")}
	svc := jobsvc.NewPipelineService(&pipelineJobWriter{}, ledger, &pipelineSnapshotWriter{}, &pipelineTechTagWriter{})
	if err := svc.MarkLedgerIrrelevant(context.Background(), "job-1"); err == nil {
		t.Fatal("expected error when ledger update fails")
	}
}

// TestSaveEnrichedJob_LedgerUpdateError_Propagates — if the post-insert
// ledger update fails, SaveEnrichedJob must surface the error. The job and
// snapshot writes already happened (best-effort), but the caller needs to
// know the ledger is out of sync.
func TestSaveEnrichedJob_LedgerUpdateError_Propagates(t *testing.T) {
	jobs := &pipelineJobWriter{}
	ledger := &pipelineLedgerStore{updateReasonErr: errors.New("ledger update fail")}
	snapshot := &pipelineSnapshotWriter{}
	svc := jobsvc.NewPipelineService(jobs, ledger, snapshot, &pipelineTechTagWriter{})

	if err := svc.SaveEnrichedJob(context.Background(), makeInput()); err == nil {
		t.Fatal("expected error when ledger update fails after insert")
	}
	// Job still inserted; snapshot still written (best-effort).
	if jobs.inserted == nil {
		t.Error("expected InsertJob to be called even when ledger fails")
	}
	if !snapshot.insertCalled {
		t.Error("expected InsertSnapshot to be called before ledger update")
	}
}

// TestSaveEnrichedJob_EmptyTechStack_NoTagInsert — when Clean produces no
// tags, the InsertTechTags call must be skipped (no empty rows written).
func TestSaveEnrichedJob_EmptyTechStack_NoTagInsert(t *testing.T) {
	tt := &countingTechTagWriter{}
	jobs := &pipelineJobWriter{}
	ledger := &pipelineLedgerStore{}
	svc := jobsvc.NewPipelineService(jobs, ledger, &pipelineSnapshotWriter{}, tt)

	input := makeInput()
	input.Enriched.TechStack = nil

	if err := svc.SaveEnrichedJob(context.Background(), input); err != nil {
		t.Fatalf("SaveEnrichedJob err = %v", err)
	}
	if tt.calls != 0 {
		t.Errorf("InsertTechTags calls = %d, want 0 (no tags after Clean)", tt.calls)
	}
}

// TestSaveEnrichedJob_TechTagsCalled — when the enricher produces tech tags,
// the tag writer fires once.
func TestSaveEnrichedJob_TechTagsCalled(t *testing.T) {
	tt := &countingTechTagWriter{}
	jobs := &pipelineJobWriter{}
	ledger := &pipelineLedgerStore{}
	svc := jobsvc.NewPipelineService(jobs, ledger, &pipelineSnapshotWriter{}, tt)

	if err := svc.SaveEnrichedJob(context.Background(), makeInput()); err != nil {
		t.Fatalf("SaveEnrichedJob err = %v", err)
	}
	if tt.calls != 1 {
		t.Errorf("InsertTechTags calls = %d, want 1", tt.calls)
	}
}
