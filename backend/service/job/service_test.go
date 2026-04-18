package job

import (
	"context"
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	"gorm.io/gorm"
)

// --- Test doubles ---

type fakeJobStore struct {
	job   *model.Job
	jobs  []model.Job
	total int64
	err   error

	updateStatusCalled bool
	updateSavedCalled  bool
	updateNotesCalled  bool
}

func (f *fakeJobStore) GetJobs(_ context.Context, _ JobFilter, _, _ int) ([]model.Job, int64, error) {
	return f.jobs, f.total, f.err
}
func (f *fakeJobStore) GetJobByID(_ context.Context, id string) (*model.Job, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.job != nil && f.job.ID == id {
		return f.job, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (f *fakeJobStore) InsertJob(_ context.Context, job *model.Job) error { return f.err }
func (f *fakeJobStore) DeleteJob(_ context.Context, id string) error      { return f.err }
func (f *fakeJobStore) UpdateJobStatus(_ context.Context, id, status string) error {
	f.updateStatusCalled = true
	return f.err
}
func (f *fakeJobStore) UpdateJobSaved(_ context.Context, id string, saved bool) error {
	f.updateSavedCalled = true
	return f.err
}
func (f *fakeJobStore) UpdateJobNotes(_ context.Context, id, notes string) error {
	f.updateNotesCalled = true
	return f.err
}
func (f *fakeJobStore) GetJobCounts(_ context.Context) (*JobCounts, error) {
	return &JobCounts{}, f.err
}
func (f *fakeJobStore) GetJobsAddedByDay(_ context.Context, days int) ([]DailyJobCount, error) {
	return nil, f.err
}

type fakeLedgerWriter struct {
	err error
}

func (f *fakeLedgerWriter) MarkSeen(_ context.Context, jobID, companyID, reason string) error {
	return f.err
}
func (f *fakeLedgerWriter) UpdateSeenReason(_ context.Context, jobID, reason string) error {
	return f.err
}
func (f *fakeLedgerWriter) UpdateSeenReasonBatch(_ context.Context, ids []string, reason string) error {
	return f.err
}
func (f *fakeLedgerWriter) MarkRemovedBatch(_ context.Context, ids []string) error { return f.err }

type fakeDiscardStore struct {
	err          error
	insertCalled bool
}

func (f *fakeDiscardStore) InsertDiscard(_ context.Context, jobResultID, reason, note string) error {
	f.insertCalled = true
	return f.err
}

type fakeSnapshotWriter struct {
	err          error
	updateCalled bool
}

func (f *fakeSnapshotWriter) InsertSnapshot(_ context.Context, input SnapshotInput) error {
	return f.err
}
func (f *fakeSnapshotWriter) UpdateSnapshotAction(_ context.Context, id, action string) error {
	f.updateCalled = true
	return f.err
}

// --- Tests ---

func TestList_ReturnsPaged(t *testing.T) {
	jobs := []model.Job{{ID: "j1"}, {ID: "j2"}}
	svc := NewService(&fakeJobStore{jobs: jobs, total: 2}, &fakeLedgerWriter{}, &fakeDiscardStore{}, &fakeSnapshotWriter{})

	result, total, err := svc.List(context.Background(), JobFilter{Status: "new"}, 1, 20)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(result))
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
}

func TestGet_NotFound(t *testing.T) {
	svc := NewService(&fakeJobStore{}, &fakeLedgerWriter{}, &fakeDiscardStore{}, &fakeSnapshotWriter{})

	_, err := svc.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing job")
	}
	var sErr *domain.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND error, got %v", err)
	}
}

func TestDismiss_MultiStep(t *testing.T) {
	job := &model.Job{ID: "j1", JobPostID: "jp1"}
	js := &fakeJobStore{job: job}
	discard := &fakeDiscardStore{}
	snapshot := &fakeSnapshotWriter{}
	ledger := &fakeLedgerWriter{}

	svc := NewService(js, ledger, discard, snapshot)
	err := svc.Dismiss(context.Background(), "j1", DismissInput{Reason: model.DiscardReasonDismissed})
	if err != nil {
		t.Fatalf("Dismiss() error = %v", err)
	}

	if !discard.insertCalled {
		t.Error("expected InsertDiscard to be called")
	}
	if !js.updateStatusCalled {
		t.Error("expected UpdateJobStatus to be called")
	}
	// snapshot update is non-fatal and always attempted
	if !snapshot.updateCalled {
		t.Error("expected UpdateSnapshotAction to be called")
	}
}

func TestSetSaved_NonFatalSnapshot(t *testing.T) {
	js := &fakeJobStore{}
	snapshot := &fakeSnapshotWriter{err: errors.New("snapshot error")} // non-fatal error
	svc := NewService(js, &fakeLedgerWriter{}, &fakeDiscardStore{}, snapshot)

	err := svc.SetSaved(context.Background(), "j1", true)
	if err != nil {
		t.Fatalf("SetSaved() should not return error even if snapshot fails, got: %v", err)
	}
	if !js.updateSavedCalled {
		t.Error("expected UpdateJobSaved to be called")
	}
}

func TestSetStatus_BackfillsSnapshotOnApplied(t *testing.T) {
	js := &fakeJobStore{}
	snapshot := &fakeSnapshotWriter{}
	svc := NewService(js, &fakeLedgerWriter{}, &fakeDiscardStore{}, snapshot)

	err := svc.SetStatus(context.Background(), "j1", model.JobStatusApplied)
	if err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	if !snapshot.updateCalled {
		t.Error("expected UpdateSnapshotAction to be called for applied status")
	}
}

func TestSetStatus_NoSnapshotBackfillForNew(t *testing.T) {
	js := &fakeJobStore{}
	snapshot := &fakeSnapshotWriter{}
	svc := NewService(js, &fakeLedgerWriter{}, &fakeDiscardStore{}, snapshot)

	err := svc.SetStatus(context.Background(), "j1", model.JobStatusNew)
	if err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	if snapshot.updateCalled {
		t.Error("expected UpdateSnapshotAction NOT to be called for new status")
	}
}

// Ensure test doubles implement interfaces.
var _ JobStore = (*fakeJobStore)(nil)
var _ SeenLedgerWriter = (*fakeLedgerWriter)(nil)
var _ DiscardStore = (*fakeDiscardStore)(nil)
var _ SnapshotWriter = (*fakeSnapshotWriter)(nil)
