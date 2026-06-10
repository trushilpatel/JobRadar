package services_test

import (
	"context"
	stderrors "errors"
	"testing"

	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// errorSnapshot returns errors from all mutation methods so the Warn logging
// paths in ImplService fire without aborting the primary operation.
type errorSnapshot struct{}

func (errorSnapshot) InsertSnapshot(_ context.Context, _ jobdto.SnapshotInput) error { return nil }
func (errorSnapshot) UpdateSnapshotAction(_ context.Context, _, _ string) error {
	return stderrors.New("snapshot error")
}
func (errorSnapshot) UpdateSnapshotActionBatch(_ context.Context, _ []string, _ string) error {
	return stderrors.New("snapshot batch error")
}

// errorAppWriter always returns an error.
type errorAppWriter struct{}

func (errorAppWriter) InsertAppliedApplication(_ context.Context, _ string) error {
	return stderrors.New("app writer error")
}

// errorLedger returns errors from UpdateSeenReason and UpdateSeenReasonBatch.
// It embeds *fakeLedgerWriter so pointer-receiver methods (MarkRemovedBatch)
// are promoted correctly.
type errorLedger struct{ *fakeLedgerWriter }

func (errorLedger) UpdateSeenReason(_ context.Context, _ string, _ jobcommon.SeenReason) error {
	return stderrors.New("ledger error")
}
func (errorLedger) UpdateSeenReasonBatch(_ context.Context, _ []string, _ jobcommon.SeenReason) error {
	return stderrors.New("ledger batch error")
}

// TestImpl_SetSaved_SnapshotWarn — when snapshot.UpdateSnapshotAction fails on
// SetSaved(true), a warn is logged but the call succeeds.
func TestImpl_SetSaved_SnapshotWarn(t *testing.T) {
	log := &recordingLogger{}
	svc := jobsvc.NewImplService(&fakeJobReader{}, &fakeUserState{}, errorSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, log)
	if err := svc.SetSaved(ctxAs("u1"), "j1", true); err != nil {
		t.Fatalf("SetSaved err = %v", err)
	}
	if log.warnCount == 0 {
		t.Error("expected warn for snapshot error")
	}
}

// TestImpl_SetStatus_SavedGuard — the impl rejects "saved" as a workflow
// status; callers must use SetSaved instead.
func TestImpl_SetStatus_SavedGuard(t *testing.T) {
	svc := jobsvc.NewImplService(&fakeJobReader{}, &fakeUserState{}, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)
	err := svc.SetStatus(ctxAs("u1"), "j1", jobcommon.JobStatusSaved)
	if err == nil {
		t.Fatal("expected BAD_REQUEST for 'saved' status")
	}
}

// TestImpl_SetStatus_Applied_SnapshotAndAppWriterWarn — when snapshot and
// appWriter fail on applied, both warn and the call still returns nil.
func TestImpl_SetStatus_Applied_SnapshotAndAppWriterWarn(t *testing.T) {
	log := &recordingLogger{}
	svc := jobsvc.NewImplService(&fakeJobReader{}, &fakeUserState{}, errorSnapshot{}, errorAppWriter{}, &fakeLedgerWriter{}, nil, log)
	if err := svc.SetStatus(ctxAs("u1"), "j1", jobmodel.StatusApplied); err != nil {
		t.Fatalf("SetStatus err = %v", err)
	}
	if log.warnCount < 2 {
		t.Errorf("expected ≥2 warns (snapshot + appWriter), got %d", log.warnCount)
	}
}

// TestImpl_SetStatus_Dismissed_SnapshotWarn — snapshot warn on dismissed.
func TestImpl_SetStatus_Dismissed_SnapshotWarn(t *testing.T) {
	log := &recordingLogger{}
	svc := jobsvc.NewImplService(&fakeJobReader{}, &fakeUserState{}, errorSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, log)
	if err := svc.SetStatus(ctxAs("u1"), "j1", jobmodel.StatusDismissed); err != nil {
		t.Fatalf("SetStatus err = %v", err)
	}
	if log.warnCount == 0 {
		t.Error("expected warn for dismissed snapshot error")
	}
}

// TestImpl_SetNotes_DefaultCaseSavedOnlyRow — when the row exists with nil
// status (saved-only), SetNotes uses JobStatusNew sentinel (default switch
// branch, line 332).
func TestImpl_SetNotes_DefaultCaseSavedOnlyRow(t *testing.T) {
	existing := &jobmodel.UserJobState{UserID: "u1", JobID: "j1"} // Status = nil
	state := &fakeUserState{getResult: existing}
	svc := jobsvc.NewImplService(&fakeJobReader{}, state, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)
	if err := svc.SetNotes(ctxAs("u1"), "j1", "some notes"); err != nil {
		t.Fatalf("SetNotes err = %v", err)
	}
	if state.upsertState.Status != "new" {
		t.Errorf("upsert status = %q, want new (saved-only row)", state.upsertState.Status)
	}
}

// TestImpl_SetNotes_UpsertError — when Upsert fails, SetNotes propagates it.
func TestImpl_SetNotes_UpsertError(t *testing.T) {
	state := &fakeUserState{upsertErr: stderrors.New("upsert fail")}
	svc := jobsvc.NewImplService(&fakeJobReader{}, state, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)
	if err := svc.SetNotes(ctxAs("u1"), "j1", "notes"); err == nil {
		t.Fatal("expected error from Upsert failure")
	}
}

// TestImpl_Dismiss_LedgerAndSnapshotWarn — when ledger and snapshot fail
// inside Dismiss, both warn and the call still returns nil.
func TestImpl_Dismiss_LedgerAndSnapshotWarn(t *testing.T) {
	log := &recordingLogger{}
	reader := &fakeJobReader{job: &jobmodel.Job{ID: "j1", JobPostID: "post-1"}}
	svc := jobsvc.NewImplService(reader, &fakeUserState{}, errorSnapshot{}, &fakeAppWriter{}, errorLedger{&fakeLedgerWriter{}}, nil, log)
	if err := svc.Dismiss(ctxAs("u1"), "j1", jobcommon.DismissalReasonNotRelevant, ""); err != nil {
		t.Fatalf("Dismiss err = %v", err)
	}
	if log.warnCount < 2 {
		t.Errorf("expected ≥2 warns (ledger + snapshot), got %d", log.warnCount)
	}
}

// TestImpl_BulkDismiss_LedgerAndSnapshotWarn — both best-effort side effects
// can fail without aborting the BulkDismiss call.
func TestImpl_BulkDismiss_LedgerAndSnapshotWarn(t *testing.T) {
	log := &recordingLogger{}
	svc := jobsvc.NewImplService(&fakeJobReader{}, &fakeUserState{}, errorSnapshot{}, &fakeAppWriter{}, errorLedger{&fakeLedgerWriter{}}, nil, log)
	if err := svc.BulkDismiss(ctxAs("u1"), []string{"j1", "j2"}, jobcommon.DismissalReasonNotRelevant, ""); err != nil {
		t.Fatalf("BulkDismiss err = %v", err)
	}
	if log.warnCount < 2 {
		t.Errorf("expected ≥2 warns (ledger batch + snapshot batch), got %d", log.warnCount)
	}
}

// TestImpl_List_StatusTab_OverlayFailure — when GetMany fails inside
// listByUserStatus, a warn is logged and the jobs are returned with default
// overlay.
func TestImpl_List_StatusTab_OverlayFailure(t *testing.T) {
	log := &recordingLogger{}
	state := &fakeUserState{
		statusJobs: []jobmodel.Job{{ID: "j1"}},
		manyErr:    stderrors.New("getmany db error"),
	}
	svc := jobsvc.NewImplService(&fakeJobReader{}, state, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, log)
	res, err := svc.List(ctxAs("u1"), jobdto.JobFilter{Status: jobmodel.StatusApplied}, "", 10)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(res.Items) != 1 {
		t.Errorf("want 1 item, got %d", len(res.Items))
	}
	if log.warnCount == 0 {
		t.Error("expected warn for overlay GetMany failure in status tab")
	}
}
