package services_test

import (
	"context"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
)

// Mutation-method tests (SetSaved/SetStatus/SetNotes) split out of
// JobImplService_test.go to keep hot-path test files under the 800-line
// ergonomics threshold. Shared fixtures (fakeJobReader, fakeUserState,
// newSvc, ctxAs, strPtr, ...) live in JobImplService_test.go — same package,
// no import needed.

func TestImpl_SetSaved_RequiresUserID(t *testing.T) {
	svc := newSvc(&fakeJobReader{}, &fakeUserState{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	if err := svc.SetSaved(context.Background(), "j1", true); err == nil {
		t.Fatal("expected error without identity in ctx")
	}
}

func TestImpl_SetSaved_RoutesThroughSetSaved(t *testing.T) {
	// SetSaved now delegates to the dedicated repo SetSaved path instead of
	// Upsert+Delete; saved is orthogonal to workflow status.
	state := &fakeUserState{}
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	if err := svc.SetSaved(ctxAs("u1"), "j1", false); err != nil {
		t.Fatalf("SetSaved err = %v", err)
	}
	if state.setSavedCalls != 1 {
		t.Errorf("expected repo.SetSaved called once, got %d", state.setSavedCalls)
	}
	if state.lastSetSavedBool != false {
		t.Errorf("lastSetSavedBool = %v, want false", state.lastSetSavedBool)
	}
	if state.deleteCalls != 0 || state.upsertCalls != 0 {
		t.Errorf("legacy paths invoked — delete=%d upsert=%d", state.deleteCalls, state.upsertCalls)
	}
}

func TestImpl_SetStatus_AppliedTriggersAppWriter(t *testing.T) {
	state := &fakeUserState{}
	app := &fakeAppWriter{}
	svc := newSvc(&fakeJobReader{}, state, app, &fakeLedgerWriter{}, nil)
	if err := svc.SetStatus(ctxAs("u1"), "j1", jobmodel.StatusApplied); err != nil {
		t.Fatalf("SetStatus err = %v", err)
	}
	if app.calls != 1 {
		t.Errorf("expected app writer to be called, got %d", app.calls)
	}
	if state.upsertState.Status != jobmodel.StatusApplied {
		t.Errorf("upsert status = %q, want %q", state.upsertState.Status, jobmodel.StatusApplied)
	}
}

// -------- Missing user-id rejection --------

// TestImpl_MutationsRequireUserID asserts every mutating method errors when
// the context carries no identity. This is the defence-in-depth against a
// forgotten ctx scoping; it complements the per-repo userScoped guards.
func TestImpl_MutationsRequireUserID(t *testing.T) {
	svc := newSvc(&fakeJobReader{}, &fakeUserState{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	ctx := context.Background()
	tests := []struct {
		name string
		call func() error
	}{
		{name: "SetSaved", call: func() error { return svc.SetSaved(ctx, "j1", true) }},
		{name: "SetStatus", call: func() error { return svc.SetStatus(ctx, "j1", "applied") }},
		{name: "SetNotes", call: func() error { return svc.SetNotes(ctx, "j1", "n") }},
		{name: "GetCounts", call: func() error { _, err := svc.GetCounts(ctx); return err }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatalf("%s with no identity must error", tc.name)
			}
		})
	}
}

// -------- SetNotes status preservation --------

func TestImpl_SetNotes_ExistingRowPreservesStatus(t *testing.T) {
	state := &fakeUserState{
		getResult: &jobmodel.UserJobState{Status: strPtr(jobmodel.StatusApplied)},
	}
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	if err := svc.SetNotes(ctxAs("u1"), "j1", "my note"); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}
	if state.upsertState.Status != jobmodel.StatusApplied {
		t.Errorf("status preserved = %q, want applied", state.upsertState.Status)
	}
	if state.upsertState.Notes == nil || *state.upsertState.Notes != "my note" {
		t.Errorf("notes = %v, want &\"my note\"", state.upsertState.Notes)
	}
}

func TestImpl_SetNotes_MissingRowDefaultsToSaved(t *testing.T) {
	state := &fakeUserState{
		getErr: apperr.ErrNotFound.WithMessage("not found"),
	}
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	if err := svc.SetNotes(ctxAs("u1"), "j1", "first note"); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}
	if state.upsertState.Status != jobmodel.StatusSaved {
		t.Errorf("default status = %q, want saved", state.upsertState.Status)
	}
}
