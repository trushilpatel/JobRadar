package tests

import (
	"context"
	"testing"

	applyqueuedto "github.com/trushilpatel/jobradar/service/applyqueue/dto"
	applyqueuesvc "github.com/trushilpatel/jobradar/service/applyqueue/services"
)

// recordingService captures every call so BaseService delegation tests can
// assert each pass-through hits the next layer, mirroring
// service/job/tests/JobBaseService_test.go's convention.
type recordingService struct {
	listCalls       int
	lastListUserID  string
	lastListFilter  string
	lastListCursor  string
	lastListLimit   int
	setResumeCalls  int
	lastSetUserID   string
	lastSetJobID    string
	lastSetResumeID *string
}

func (r *recordingService) List(_ context.Context, userID, filter, cursor string, limit int) (applyqueuedto.QueueListResult, error) {
	r.listCalls++
	r.lastListUserID = userID
	r.lastListFilter = filter
	r.lastListCursor = cursor
	r.lastListLimit = limit
	return applyqueuedto.QueueListResult{NextCursor: "delegated"}, nil
}

func (r *recordingService) SetResume(_ context.Context, userID, jobID string, resumeID *string) (*applyqueuedto.AttachmentView, error) {
	r.setResumeCalls++
	r.lastSetUserID = userID
	r.lastSetJobID = jobID
	r.lastSetResumeID = resumeID
	return &applyqueuedto.AttachmentView{JobID: jobID}, nil
}

var _ applyqueuesvc.Service = (*recordingService)(nil)

// passThroughService embeds BaseService verbatim — any method not overridden
// delegates to next.
type passThroughService struct {
	applyqueuesvc.BaseService
}

func TestBaseService_PanicsOnNilNext(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil next")
		}
	}()
	_ = applyqueuesvc.NewBaseService(nil)
}

func TestBaseService_DelegatesEveryMethod(t *testing.T) {
	next := &recordingService{}
	pt := &passThroughService{BaseService: applyqueuesvc.NewBaseService(next)}
	ctx := context.Background()

	result, err := pt.List(ctx, "u1", "all", "tok", 25)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if result.NextCursor != "delegated" {
		t.Errorf("NextCursor = %q, want %q", result.NextCursor, "delegated")
	}
	if next.listCalls != 1 {
		t.Errorf("listCalls = %d, want 1", next.listCalls)
	}
	if next.lastListUserID != "u1" || next.lastListFilter != "all" || next.lastListCursor != "tok" || next.lastListLimit != 25 {
		t.Errorf("List forwarded args wrong: userID=%q filter=%q cursor=%q limit=%d",
			next.lastListUserID, next.lastListFilter, next.lastListCursor, next.lastListLimit)
	}

	resumeID := "r1"
	view, err := pt.SetResume(ctx, "u1", "j1", &resumeID)
	if err != nil {
		t.Fatalf("SetResume err = %v", err)
	}
	if view == nil || view.JobID != "j1" {
		t.Errorf("SetResume view = %+v, want job_id=j1", view)
	}
	if next.setResumeCalls != 1 {
		t.Errorf("setResumeCalls = %d, want 1", next.setResumeCalls)
	}
	if next.lastSetUserID != "u1" || next.lastSetJobID != "j1" || next.lastSetResumeID != &resumeID {
		t.Errorf("SetResume forwarded args wrong: userID=%q jobID=%q resumeID=%v",
			next.lastSetUserID, next.lastSetJobID, next.lastSetResumeID)
	}
}
