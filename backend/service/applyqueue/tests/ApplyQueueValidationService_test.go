package tests

import (
	"context"
	"errors"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	applyqueuecommon "github.com/trushilpatel/jobradar/service/applyqueue/common"
	applyqueuesvc "github.com/trushilpatel/jobradar/service/applyqueue/services"
)

// stubEngagementChecker returns a fixed answer (or error) for every call.
type stubEngagementChecker struct {
	has bool
	err error
}

func (s *stubEngagementChecker) HasEngagement(_ context.Context, _, _ string) (bool, error) {
	return s.has, s.err
}

// stubResumeOwnershipChecker returns a fixed answer (or error) for every call.
type stubResumeOwnershipChecker struct {
	owned bool
	err   error
}

func (s *stubResumeOwnershipChecker) ResumeOwnedByUser(_ context.Context, _, _ string) (bool, error) {
	return s.owned, s.err
}

func newValidationServiceForTest(next *recordingService, has bool, owned bool) applyqueuesvc.Service {
	return applyqueuesvc.NewValidationService(next, &stubEngagementChecker{has: has}, &stubResumeOwnershipChecker{owned: owned})
}

func TestValidation_List_DefaultsEmptyFilterToAll(t *testing.T) {
	next := &recordingService{}
	v := newValidationServiceForTest(next, true, true)

	if _, err := v.List(context.Background(), "u1", "", "", 10); err != nil {
		t.Fatalf("List err = %v", err)
	}
	if next.lastListFilter != applyqueuecommon.FilterAll {
		t.Errorf("filter = %q, want %q", next.lastListFilter, applyqueuecommon.FilterAll)
	}
}

func TestValidation_List_RejectsUnknownFilter(t *testing.T) {
	next := &recordingService{}
	v := newValidationServiceForTest(next, true, true)

	_, err := v.List(context.Background(), "u1", "bogus", "", 10)
	if err == nil {
		t.Fatal("expected validation error for unknown filter")
	}
	var vErrs domainerrs.ValidationErrors
	if !errors.As(err, &vErrs) {
		t.Fatalf("expected ValidationErrors, got %T: %v", err, err)
	}
	if next.listCalls != 0 {
		t.Errorf("expected next.List not called on validation failure, got %d calls", next.listCalls)
	}
}

func TestValidation_List_AcceptsSavedFilter(t *testing.T) {
	next := &recordingService{}
	v := newValidationServiceForTest(next, true, true)

	if _, err := v.List(context.Background(), "u1", applyqueuecommon.FilterSaved, "", 10); err != nil {
		t.Fatalf("List err = %v", err)
	}
	if next.lastListFilter != applyqueuecommon.FilterSaved {
		t.Errorf("filter = %q, want %q", next.lastListFilter, applyqueuecommon.FilterSaved)
	}
}

func TestValidation_SetResume_RejectsEmptyJobID(t *testing.T) {
	next := &recordingService{}
	v := newValidationServiceForTest(next, true, true)

	_, err := v.SetResume(context.Background(), "u1", "", nil)
	if err == nil {
		t.Fatal("expected validation error for empty job id")
	}
	var vErrs domainerrs.ValidationErrors
	if !errors.As(err, &vErrs) {
		t.Fatalf("expected ValidationErrors, got %T: %v", err, err)
	}
}

func TestValidation_SetResume_RejectsNoEngagement(t *testing.T) {
	next := &recordingService{}
	v := newValidationServiceForTest(next, false, true)

	_, err := v.SetResume(context.Background(), "u1", "j1", nil)
	if err == nil {
		t.Fatal("expected validation error when no user_job_state row exists")
	}
	var vErrs domainerrs.ValidationErrors
	if !errors.As(err, &vErrs) {
		t.Fatalf("expected ValidationErrors, got %T: %v", err, err)
	}
	if next.setResumeCalls != 0 {
		t.Errorf("expected next.SetResume not called, got %d calls", next.setResumeCalls)
	}
}

func TestValidation_SetResume_EngagementCheckError_Propagates(t *testing.T) {
	next := &recordingService{}
	boom := domainerrs.ErrInternal.WithMessage("boom")
	v := applyqueuesvc.NewValidationService(next, &stubEngagementChecker{err: boom}, &stubResumeOwnershipChecker{})

	_, err := v.SetResume(context.Background(), "u1", "j1", nil)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want propagated %v", err, boom)
	}
}

func TestValidation_SetResume_ClearingDoesNotCheckOwnership(t *testing.T) {
	next := &recordingService{}
	// owned=false would fail if the ownership check ran — clearing (nil
	// resumeID) must skip it entirely.
	v := newValidationServiceForTest(next, true, false)

	if _, err := v.SetResume(context.Background(), "u1", "j1", nil); err != nil {
		t.Fatalf("SetResume err = %v", err)
	}
	if next.setResumeCalls != 1 {
		t.Errorf("expected next.SetResume called once, got %d", next.setResumeCalls)
	}
}

func TestValidation_SetResume_RejectsEmptyResumeIDString(t *testing.T) {
	next := &recordingService{}
	v := newValidationServiceForTest(next, true, true)
	empty := ""

	_, err := v.SetResume(context.Background(), "u1", "j1", &empty)
	if err == nil {
		t.Fatal("expected validation error for empty resume_id string")
	}
	var vErrs domainerrs.ValidationErrors
	if !errors.As(err, &vErrs) {
		t.Fatalf("expected ValidationErrors, got %T: %v", err, err)
	}
}

func TestValidation_SetResume_RejectsResumeNotOwned(t *testing.T) {
	next := &recordingService{}
	v := newValidationServiceForTest(next, true, false)
	resumeID := "r1"

	_, err := v.SetResume(context.Background(), "u1", "j1", &resumeID)
	if !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if next.setResumeCalls != 0 {
		t.Errorf("expected next.SetResume not called, got %d", next.setResumeCalls)
	}
}

func TestValidation_SetResume_OwnershipCheckError_Propagates(t *testing.T) {
	next := &recordingService{}
	boom := domainerrs.ErrInternal.WithMessage("boom")
	v := applyqueuesvc.NewValidationService(next, &stubEngagementChecker{has: true}, &stubResumeOwnershipChecker{err: boom})
	resumeID := "r1"

	_, err := v.SetResume(context.Background(), "u1", "j1", &resumeID)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want propagated %v", err, boom)
	}
}

func TestValidation_SetResume_HappyPath_Attach_DelegatesToNext(t *testing.T) {
	next := &recordingService{}
	v := newValidationServiceForTest(next, true, true)
	resumeID := "r1"

	view, err := v.SetResume(context.Background(), "u1", "j1", &resumeID)
	if err != nil {
		t.Fatalf("SetResume err = %v", err)
	}
	if view == nil || view.JobID != "j1" {
		t.Errorf("view = %+v, want job_id=j1", view)
	}
	if next.setResumeCalls != 1 {
		t.Errorf("expected next.SetResume called once, got %d", next.setResumeCalls)
	}
}
