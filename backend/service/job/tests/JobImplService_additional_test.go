package services_test

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	"gorm.io/gorm"
)

// TestImpl_GetCompanies_Delegates verifies GetCompanies pass-through to the
// JobReader. Previously uncovered — the simplest delegation in the impl.
func TestImpl_GetCompanies_Delegates(t *testing.T) {
	wantNames := []string{"Acme", "Globex"}
	reader := &fakeJobReader{distinctNames: wantNames}
	svc := jobsvc.NewImplService(reader, &fakeUserState{}, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)

	names, err := svc.GetCompanies(context.Background())
	if err != nil {
		t.Fatalf("GetCompanies err = %v", err)
	}
	if len(names) != 2 || names[0] != "Acme" || names[1] != "Globex" {
		t.Errorf("names = %v, want [Acme Globex]", names)
	}
}

// TestImpl_Get_ErrInternalOnGenericReadFailure asserts non-NotFound errors
// surface as ErrInternal — guards against leaking driver-specific messages.
func TestImpl_Get_ErrInternalOnGenericReadFailure(t *testing.T) {
	reader := &fakeJobReader{getByIDErr: stderrors.New("boom")}
	svc := jobsvc.NewImplService(reader, &fakeUserState{}, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)

	_, err := svc.Get(ctxAs("u1"), "j1")
	var se *apperr.ServiceError
	if !stderrors.As(err, &se) {
		t.Fatalf("err = %v, want *ServiceError", err)
	}
	if se.Code != "INTERNAL" {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// TestImpl_Get_OverlayPopulatedWithTimestamps checks that toUserStateView's
// pointer-time-formatting branches fire. Prior tests only exercised the nil
// pointer paths.
func TestImpl_Get_OverlayPopulatedWithTimestamps(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	notes := "did something"
	dr := jobcommon.DismissalReasonNotRelevant
	dn := "stale"
	applied := jobmodel.StatusApplied
	state := &fakeUserState{getResult: &jobmodel.UserJobState{
		UserID:          "u1",
		JobID:           "j1",
		Status:          &applied,
		Notes:           &notes,
		DismissalReason: &dr,
		DismissalNote:   &dn,
		SavedAt:         &now,
		AppliedAt:       &now,
		DismissedAt:     &now,
	}}
	reader := &fakeJobReader{job: &jobmodel.Job{ID: "j1"}}
	svc := jobsvc.NewImplService(reader, state, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)

	view, err := svc.Get(ctxAs("u1"), "j1")
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	if view.UserState.Status != jobmodel.StatusApplied {
		t.Errorf("status = %q, want applied", view.UserState.Status)
	}
	if view.UserState.SavedAt == nil || *view.UserState.SavedAt == "" {
		t.Errorf("SavedAt empty, want RFC3339 string")
	}
	if view.UserState.AppliedAt == nil || view.UserState.DismissedAt == nil {
		t.Error("expected AppliedAt + DismissedAt populated")
	}
	if view.UserState.Notes == nil || *view.UserState.Notes != notes {
		t.Errorf("Notes = %v, want %q", view.UserState.Notes, notes)
	}
}

// TestImpl_Get_OverlayUnexpectedErrorTriggersWarnPath — when userState.Get
// returns a non-NOT_FOUND error, the warn-logging branch executes. The fake
// logger swallows the call; we just need to verify the impl doesn't crash and
// returns the default "new" status overlay.
func TestImpl_Get_OverlayUnexpectedErrorTriggersWarnPath(t *testing.T) {
	reader := &fakeJobReader{job: &jobmodel.Job{ID: "j1"}}
	state := &fakeUserState{getErr: stderrors.New("transient db hiccup")}
	svc := jobsvc.NewImplService(reader, state, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, &recordingLogger{})

	view, err := svc.Get(ctxAs("u1"), "j1")
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	if view.UserState.Status != jobcommon.JobStatusNew {
		t.Errorf("status = %q, want new (fallback after warn)", view.UserState.Status)
	}
}

// TestImpl_List_DBErrorPropagates — when the reader returns a non-typed
// error, List converts to *ServiceError(INTERNAL).
func TestImpl_List_DBErrorPropagates(t *testing.T) {
	reader := &fakeJobReader{err: stderrors.New("connection refused")}
	svc := jobsvc.NewImplService(reader, &fakeUserState{}, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)
	_, err := svc.List(ctxAs("u1"), jobdto.JobFilter{}, "", 10)
	var se *apperr.ServiceError
	if !stderrors.As(err, &se) {
		t.Fatalf("err = %v, want *ServiceError", err)
	}
	if se.Code != "INTERNAL" {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// TestImpl_List_StatusTab_PropagatesErrAsInternal — when GetJobsByStatus
// fails, the impl wraps as ErrInternal.
func TestImpl_List_StatusTab_PropagatesErrAsInternal(t *testing.T) {
	state := &fakeUserState{statusErr: stderrors.New("oops")}
	svc := jobsvc.NewImplService(&fakeJobReader{}, state, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)
	_, err := svc.List(ctxAs("u1"), jobdto.JobFilter{Status: jobmodel.StatusSaved}, "", 10)
	var se *apperr.ServiceError
	if !stderrors.As(err, &se) {
		t.Fatalf("err = %v, want ServiceError", err)
	}
	if se.Code != "INTERNAL" {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// TestImpl_List_OverlayFailure_FallsBackToEmptyOverlay — when GetMany fails,
// the impl logs and returns the page with the default "new" overlay. No error
// to the caller.
func TestImpl_List_OverlayFailure_FallsBackToEmptyOverlay(t *testing.T) {
	reader := &fakeJobReader{jobs: []jobmodel.Job{{ID: "j1"}, {ID: "j2"}}}
	state := &fakeUserState{manyErr: stderrors.New("overlay db down")}
	log := &recordingLogger{}
	svc := jobsvc.NewImplService(reader, state, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, log)

	res, err := svc.List(ctxAs("u1"), jobdto.JobFilter{}, "", 10)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("want 2 items even after overlay failure, got %d", len(res.Items))
	}
	for _, it := range res.Items {
		if it.UserState.Status != jobcommon.JobStatusNew {
			t.Errorf("item %s status = %q, want new (overlay fallback)", it.ID, it.UserState.Status)
		}
	}
	if log.warnCount == 0 {
		t.Error("expected at least one warn log for overlay fetch failure")
	}
}

// TestImpl_BulkDismiss_StoreError_Propagates — when UpsertBulkDismiss fails,
// the impl returns the underlying error without proceeding to ledger updates.
func TestImpl_BulkDismiss_StoreError_Propagates(t *testing.T) {
	state := &fakeUserState{}
	state.upsertErr = stderrors.New("write fail")
	// Wire fake state that also fails UpsertBulkDismiss — extend the fake.
	state2 := &bulkFailUserState{fakeUserState: *state}
	svc := jobsvc.NewImplService(&fakeJobReader{}, state2, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)
	err := svc.BulkDismiss(ctxAs("u1"), []string{"j1", "j2"}, jobcommon.DismissalReasonNotRelevant, "")
	if err == nil {
		t.Fatal("expected BulkDismiss to surface underlying store error")
	}
}

// TestImpl_SetSaved_StoreError_Propagates — when the repo SetSaved call
// fails, the impl surfaces the error and skips the cache invalidation.
func TestImpl_SetSaved_StoreError_Propagates(t *testing.T) {
	state := &fakeUserState{setSavedErr: stderrors.New("setsaved fail")}
	svc := jobsvc.NewImplService(&fakeJobReader{}, state, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)
	if err := svc.SetSaved(ctxAs("u1"), "j1", true); err == nil {
		t.Fatal("expected error to propagate")
	}
}

// TestImpl_SetStatus_StoreError_Propagates — when Upsert fails, side effects
// are not invoked.
func TestImpl_SetStatus_StoreError_Propagates(t *testing.T) {
	state := &fakeUserState{upsertErr: stderrors.New("oops")}
	app := &fakeAppWriter{}
	svc := jobsvc.NewImplService(&fakeJobReader{}, state, fakeSnapshot{}, app, &fakeLedgerWriter{}, nil, nil)
	if err := svc.SetStatus(ctxAs("u1"), "j1", jobmodel.StatusApplied); err == nil {
		t.Fatal("expected error from underlying upsert")
	}
	if app.calls != 0 {
		t.Errorf("appWriter.calls = %d, want 0 (skipped on upsert error)", app.calls)
	}
}

// TestImpl_Dismiss_RepoUpsertError_Propagates — when the Upsert call fails
// inside Dismiss, the error reaches the caller and side-effects are skipped.
func TestImpl_Dismiss_RepoUpsertError_Propagates(t *testing.T) {
	state := &fakeUserState{upsertErr: stderrors.New("upsert fail")}
	reader := &fakeJobReader{job: &jobmodel.Job{ID: "j1", JobPostID: "post-1"}}
	led := &fakeLedgerWriter{}
	svc := jobsvc.NewImplService(reader, state, fakeSnapshot{}, &fakeAppWriter{}, led, nil, nil)
	if err := svc.Dismiss(ctxAs("u1"), "j1", jobcommon.DismissalReasonUserDismissed, ""); err == nil {
		t.Fatal("expected error propagation")
	}
	if led.calls != 0 {
		t.Errorf("ledger.calls = %d, want 0 (side effects skipped)", led.calls)
	}
}

// TestImpl_Dismiss_GenericReadFailureReturnsInternal — Dismiss must convert
// a non-NotFound read failure into INTERNAL, not leak the raw error.
func TestImpl_Dismiss_GenericReadFailureReturnsInternal(t *testing.T) {
	reader := &fakeJobReader{getByIDErr: stderrors.New("boom")}
	svc := jobsvc.NewImplService(reader, &fakeUserState{}, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)
	err := svc.Dismiss(ctxAs("u1"), "j1", jobcommon.DismissalReasonUserDismissed, "")
	var se *apperr.ServiceError
	if !stderrors.As(err, &se) || se.Code != "INTERNAL" {
		t.Fatalf("err = %v, want INTERNAL ServiceError", err)
	}
}

// TestImpl_Dismiss_NotFoundFromRecordNotFound — domain.IsNotFound recognises
// gorm.ErrRecordNotFound; Dismiss converts to NOT_FOUND.
func TestImpl_Dismiss_NotFoundFromRecordNotFound(t *testing.T) {
	reader := &fakeJobReader{getByIDErr: gorm.ErrRecordNotFound}
	svc := jobsvc.NewImplService(reader, &fakeUserState{}, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)
	err := svc.Dismiss(ctxAs("u1"), "missing", jobcommon.DismissalReasonUserDismissed, "")
	var se *apperr.ServiceError
	if !stderrors.As(err, &se) || se.Code != "NOT_FOUND" {
		t.Fatalf("err = %v, want NOT_FOUND ServiceError", err)
	}
}

// TestImpl_Dismiss_WithNote — when note is non-empty, it is forwarded to the
// repository as a non-nil pointer.
func TestImpl_Dismiss_WithNote(t *testing.T) {
	state := &fakeUserState{}
	reader := &fakeJobReader{job: &jobmodel.Job{ID: "j1", JobPostID: "post-1"}}
	svc := jobsvc.NewImplService(reader, state, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)
	if err := svc.Dismiss(ctxAs("u1"), "j1", jobcommon.DismissalReasonNotRelevant, "stale role"); err != nil {
		t.Fatalf("Dismiss err = %v", err)
	}
	if state.upsertState.DismissalNote == nil || *state.upsertState.DismissalNote != "stale role" {
		t.Errorf("DismissalNote = %v, want \"stale role\"", state.upsertState.DismissalNote)
	}
}

// TestImpl_GetCounts_CountAllError_NewStaysZero — if CountAll fails, the
// derived New count silently stays 0 (documented best-effort fallback).
func TestImpl_GetCounts_CountAllError_NewStaysZero(t *testing.T) {
	reader := &fakeJobReader{countAllErr: stderrors.New("count failed")}
	state := &fakeUserState{countsResult: jobdto.Counts{Saved: 3}}
	svc := jobsvc.NewImplService(reader, state, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)
	got, err := svc.GetCounts(ctxAs("u1"))
	if err != nil {
		t.Fatalf("GetCounts err = %v", err)
	}
	if got.New != 0 {
		t.Errorf("New = %d, want 0 (CountAll error swallowed)", got.New)
	}
	if got.Saved != 3 {
		t.Errorf("Saved = %d, want 3", got.Saved)
	}
}

// TestImpl_GetCounts_CountsByUserError_Propagates — when the per-user counts
// query fails, the error reaches the caller (this is fatal, not best-effort).
func TestImpl_GetCounts_CountsByUserError_Propagates(t *testing.T) {
	state := &fakeUserState{countsErr: stderrors.New("counts down")}
	svc := jobsvc.NewImplService(&fakeJobReader{}, state, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)
	_, err := svc.GetCounts(ctxAs("u1"))
	if err == nil {
		t.Fatal("expected error from CountsByUser failure")
	}
}

// TestImpl_SetSaved_FalseStoreError_Propagates — when saved=false and the
// repo SetSaved call fails, the impl surfaces the error.
func TestImpl_SetSaved_FalseStoreError_Propagates(t *testing.T) {
	state := &fakeUserState{setSavedErr: stderrors.New("setsaved fail")}
	svc := jobsvc.NewImplService(&fakeJobReader{}, state, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)
	if err := svc.SetSaved(ctxAs("u1"), "j1", false); err == nil {
		t.Fatal("expected error to propagate")
	}
}

// TestImpl_invalidateUserCounts_NilCache_NoCrash — invalidate is a no-op when
// the cache is nil; the impl is wired with c=nil in many test setups.
func TestImpl_invalidateUserCounts_NilCache_NoCrash(t *testing.T) {
	state := &fakeUserState{}
	svc := jobsvc.NewImplService(&fakeJobReader{}, state, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil, nil)
	// Just call any mutation that triggers invalidate via the nil-cache path.
	if err := svc.SetSaved(ctxAs("u1"), "j1", true); err != nil {
		t.Fatalf("SetSaved err = %v", err)
	}
}

// TestImpl_invalidateUserCounts_DeleteError_LoggedNotPropagated — cache
// delete failure must NOT bubble up; it logs a warning and the mutation
// returns success.
type errDeleteCache struct {
	memHits int
}

func (e *errDeleteCache) Get(_ context.Context, _ string, _ any) (bool, error) { return false, nil }
func (e *errDeleteCache) Set(_ context.Context, _ string, _ any, _ time.Duration) error {
	return nil
}
func (e *errDeleteCache) Delete(_ context.Context, _ ...string) error {
	e.memHits++
	return stderrors.New("redis is down")
}

func TestImpl_invalidateUserCounts_DeleteError_LoggedNotPropagated(t *testing.T) {
	c := &errDeleteCache{}
	log := &recordingLogger{}
	state := &fakeUserState{}
	svc := jobsvc.NewImplService(&fakeJobReader{}, state, fakeSnapshot{}, &fakeAppWriter{}, &fakeLedgerWriter{}, c, log)
	if err := svc.SetSaved(ctxAs("u1"), "j1", true); err != nil {
		t.Fatalf("SetSaved err = %v", err)
	}
	if c.memHits == 0 {
		t.Error("expected cache.Delete to be invoked")
	}
	if log.warnCount == 0 {
		t.Error("expected at least one warn log for cache delete failure")
	}
}

// TestImpl_BulkDismiss_GetJobPostIDsError_LedgerSkipped — when the ledger
// follow-up lookup fails, the impl skips the ledger update but does NOT
// surface the error (best-effort).
func TestImpl_BulkDismiss_GetJobPostIDsError_LedgerSkipped(t *testing.T) {
	reader := &fakeJobReader{jobPostIDsErr: stderrors.New("post ids fail")}
	state := &fakeUserState{}
	led := &fakeLedgerWriter{}
	svc := jobsvc.NewImplService(reader, state, fakeSnapshot{}, &fakeAppWriter{}, led, nil, nil)
	if err := svc.BulkDismiss(ctxAs("u1"), []string{"j1"}, jobcommon.DismissalReasonNotRelevant, ""); err != nil {
		t.Fatalf("BulkDismiss err = %v", err)
	}
	// Ledger update batch is gated on a successful lookup — none here.
	if led.batchCalls != 0 {
		t.Errorf("ledger batch updates = %d, want 0 (skipped after lookup error)", led.batchCalls)
	}
}

// -------- helpers --------

type recordingLogger struct {
	warnCount int
	lastMsg   string
}

func (r *recordingLogger) Debug(_ string, _ ...logger.Field)           {}
func (r *recordingLogger) Info(_ string, _ ...logger.Field)            {}
func (r *recordingLogger) Warn(msg string, _ ...logger.Field)          { r.warnCount++; r.lastMsg = msg }
func (r *recordingLogger) Error(_ string, _ error, _ ...logger.Field)  {}
func (r *recordingLogger) With(_ ...logger.Field) logger.Logger        { return r }
func (r *recordingLogger) WithContext(_ context.Context) logger.Logger { return r }

// bulkFailUserState wraps fakeUserState with an UpsertBulkDismiss override
// that returns a configured error. Used to test the BulkDismiss store-error
// branch without touching the shared fake.
type bulkFailUserState struct {
	fakeUserState
}

func (b *bulkFailUserState) UpsertBulkDismiss(ctx context.Context, userID string, ids []string, reason, note string) error {
	if userID == "" {
		return apperr.ErrInternal.WithMessage("user id required")
	}
	return stderrors.New("bulk dismiss fail")
}

var _ jobsvc.UserJobStateRepository = (*bulkFailUserState)(nil)
