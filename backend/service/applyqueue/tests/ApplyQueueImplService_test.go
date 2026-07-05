package tests

import (
	"context"
	"testing"

	applyqueuesvc "github.com/trushilpatel/jobradar/service/applyqueue/services"
)

// TestImplService_List_DelegatesToNext confirms ImplService (currently a
// pure pass-through — see ApplyQueueImplService.go for why) forwards List
// unmodified.
func TestImplService_List_DelegatesToNext(t *testing.T) {
	next := &recordingService{}
	impl := applyqueuesvc.NewImplService(next)

	result, err := impl.List(context.Background(), "u1", "saved", "cur", 10)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if result.NextCursor != "delegated" {
		t.Errorf("NextCursor = %q, want delegated", result.NextCursor)
	}
	if next.listCalls != 1 || next.lastListUserID != "u1" || next.lastListFilter != "saved" {
		t.Errorf("List not forwarded correctly: calls=%d userID=%q filter=%q",
			next.listCalls, next.lastListUserID, next.lastListFilter)
	}
}

// TestImplService_SetResume_DelegatesToNext confirms ImplService forwards
// SetResume unmodified.
func TestImplService_SetResume_DelegatesToNext(t *testing.T) {
	next := &recordingService{}
	impl := applyqueuesvc.NewImplService(next)

	resumeID := "r1"
	view, err := impl.SetResume(context.Background(), "u1", "j1", &resumeID)
	if err != nil {
		t.Fatalf("SetResume err = %v", err)
	}
	if view == nil || view.JobID != "j1" {
		t.Errorf("view = %+v, want job_id=j1", view)
	}
	if next.setResumeCalls != 1 || next.lastSetUserID != "u1" || next.lastSetJobID != "j1" {
		t.Errorf("SetResume not forwarded correctly: calls=%d userID=%q jobID=%q",
			next.setResumeCalls, next.lastSetUserID, next.lastSetJobID)
	}
}

// TestImplService_ConstructorPanicsOnNilNext exercises the NewBaseService
// nil-guard through ImplService's constructor path.
func TestImplService_ConstructorPanicsOnNilNext(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil next")
		}
	}()
	_ = applyqueuesvc.NewImplService(nil)
}
