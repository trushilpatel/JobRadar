package services_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
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

	svc := jobsvc.NewPipelineService(jobs, ledger, snapshot, &pipelineTechTagWriter{}, logger.NewNoop())
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
	svc := jobsvc.NewPipelineService(&pipelineJobWriter{}, ledger, &pipelineSnapshotWriter{}, &pipelineTechTagWriter{}, logger.NewNoop())
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
	svc := jobsvc.NewPipelineService(jobs, ledger, snapshot, &pipelineTechTagWriter{}, logger.NewNoop())

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
	svc := jobsvc.NewPipelineService(jobs, ledger, &pipelineSnapshotWriter{}, tt, logger.NewNoop())

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
	svc := jobsvc.NewPipelineService(jobs, ledger, &pipelineSnapshotWriter{}, tt, logger.NewNoop())

	if err := svc.SaveEnrichedJob(context.Background(), makeInput()); err != nil {
		t.Fatalf("SaveEnrichedJob err = %v", err)
	}
	if tt.calls != 1 {
		t.Errorf("InsertTechTags calls = %d, want 1", tt.calls)
	}
}

// assertValidJSONArray fails unless b is non-empty and parses as a JSON
// array. An empty []byte or the JSON literal `null` is what triggers the
// production `invalid input syntax for type json (SQLSTATE 22P02)` error when
// written into a jsonb column (the `job_analytics.tech_stack` column binds a
// plain []byte, so unlike `jobs.tech_stack`'s datatypes.JSON it gets no
// empty→NULL coercion). Empty tech stacks must serialize to `[]`, never `""`
// or `null`.
func assertValidJSONArray(t *testing.T, field string, b []byte) {
	t.Helper()
	if len(b) == 0 {
		t.Fatalf("%s: empty bytes — Postgres rejects '' for a jsonb column (22P02)", field)
	}
	var arr []string
	if err := json.Unmarshal(b, &arr); err != nil {
		t.Fatalf("%s: not a valid JSON array (%q): %v", field, b, err)
	}
	if string(b) == "null" {
		t.Fatalf("%s: serialized to null, want [] for an empty stack", field)
	}
}

// TestSaveEnrichedJob_EmptyTechStack_SnapshotGetsValidJSON (regression for the
// 54×/48h `snapshot insert failed … 22P02` production error) — when the
// enriched job has NO tech stack, the bytes handed to the snapshot must be
// valid, non-empty JSON (`[]`), never empty bytes that Postgres rejects.
func TestSaveEnrichedJob_EmptyTechStack_SnapshotGetsValidJSON(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stack []string
	}{
		{"nil", nil},
		{"empty", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jobs := &pipelineJobWriter{}
			snap := &pipelineSnapshotWriter{}
			svc := jobsvc.NewPipelineService(jobs, &pipelineLedgerStore{}, snap, &pipelineTechTagWriter{}, logger.NewNoop())

			input := makeInput()
			input.Enriched.TechStack = tc.stack

			if err := svc.SaveEnrichedJob(context.Background(), input); err != nil {
				t.Fatalf("SaveEnrichedJob err = %v", err)
			}
			if !snap.insertCalled {
				t.Fatal("expected InsertSnapshot to be called")
			}
			assertValidJSONArray(t, "snapshot.TechStack", snap.insertInput.TechStack)
			if string(snap.insertInput.TechStack) != "[]" {
				t.Errorf("snapshot.TechStack = %q, want []", snap.insertInput.TechStack)
			}
			// The jobs-row JSON columns must be equally safe.
			assertValidJSONArray(t, "jobs.TechStack", jobs.inserted.TechStack)
		})
	}
}

// TestSaveEnrichedJob_MultiItemTechStack_SnapshotPreserved — a populated tech
// stack round-trips into the snapshot unchanged.
func TestSaveEnrichedJob_MultiItemTechStack_SnapshotPreserved(t *testing.T) {
	jobs := &pipelineJobWriter{}
	snap := &pipelineSnapshotWriter{}
	svc := jobsvc.NewPipelineService(jobs, &pipelineLedgerStore{}, snap, &pipelineTechTagWriter{}, logger.NewNoop())

	input := makeInput()
	input.Enriched.TechStack = []string{"Go", "PostgreSQL", "Kafka"}

	if err := svc.SaveEnrichedJob(context.Background(), input); err != nil {
		t.Fatalf("SaveEnrichedJob err = %v", err)
	}
	assertValidJSONArray(t, "snapshot.TechStack", snap.insertInput.TechStack)
	if got := string(snap.insertInput.TechStack); got != `["Go","PostgreSQL","Kafka"]` {
		t.Errorf("snapshot.TechStack = %q, want the marshalled 3-item array", got)
	}
}

// TestSaveEnrichedJob_EmptyResponsibilities_JobRowValidJSON — the other
// []string-backed json column on the jobs row must also normalize to `[]`,
// never empty bytes or `null`, guarding the same 22P02 hazard.
func TestSaveEnrichedJob_EmptyResponsibilities_JobRowValidJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		resp []string
	}{
		{"nil", nil},
		{"empty", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jobs := &pipelineJobWriter{}
			svc := jobsvc.NewPipelineService(jobs, &pipelineLedgerStore{}, &pipelineSnapshotWriter{}, &pipelineTechTagWriter{}, logger.NewNoop())

			input := makeInput()
			input.Enriched.Responsibilities = tc.resp

			if err := svc.SaveEnrichedJob(context.Background(), input); err != nil {
				t.Fatalf("SaveEnrichedJob err = %v", err)
			}
			assertValidJSONArray(t, "jobs.Responsibilities", jobs.inserted.Responsibilities)
			if string(jobs.inserted.Responsibilities) != "[]" {
				t.Errorf("jobs.Responsibilities = %q, want []", jobs.inserted.Responsibilities)
			}
		})
	}
}

// TestMarkLedgerNonUSA_HappyPath asserts the non-USA pathway writes
// SeenReasonNonUSA. This routes Indian/foreign jobs out of the active
// enrichment pipeline without counting them as errors.
func TestMarkLedgerNonUSA_HappyPath(t *testing.T) {
	jobs := &pipelineJobWriter{}
	ledger := &pipelineLedgerStore{}
	snapshot := &pipelineSnapshotWriter{}

	svc := jobsvc.NewPipelineService(jobs, ledger, snapshot, &pipelineTechTagWriter{}, logger.NewNoop())
	if err := svc.MarkLedgerNonUSA(context.Background(), "job-india-1"); err != nil {
		t.Fatalf("MarkLedgerNonUSA err = %v", err)
	}
	if ledger.updateReasonJobID != "job-india-1" {
		t.Errorf("ledger job ID = %q, want %q", ledger.updateReasonJobID, "job-india-1")
	}
	if ledger.updateReasonVal != jobcommon.SeenReasonNonUSA {
		t.Errorf("ledger reason = %q, want %q", ledger.updateReasonVal, jobcommon.SeenReasonNonUSA)
	}
}

// TestMarkLedgerNonUSA_PropagatesLedgerError — when the ledger update fails,
// the error bubbles up so the worker can handle it.
func TestMarkLedgerNonUSA_PropagatesLedgerError(t *testing.T) {
	ledger := &pipelineLedgerStore{updateReasonErr: errors.New("ledger down")}
	svc := jobsvc.NewPipelineService(&pipelineJobWriter{}, ledger, &pipelineSnapshotWriter{}, &pipelineTechTagWriter{}, logger.NewNoop())
	if err := svc.MarkLedgerNonUSA(context.Background(), "job-1"); err == nil {
		t.Fatal("expected error when ledger update fails")
	}
}
