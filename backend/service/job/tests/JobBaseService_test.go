package services_test

import (
	"context"
	"testing"

	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// recordingService captures every call so BaseService delegation tests can
// assert each pass-through hits the next layer.
type recordingService struct {
	listCalls      int
	getCalls       int
	setSavedCalls  int
	setStatusCalls int
	setNotesCalls  int
	countsCalls    int
	companiesCalls int
	lastID         string
	lastStatus     string
	lastNotes      string
	lastSavedBool  bool
	lastSavedID    string
	lastListCursor string
	lastListLimit  int
	lastListFilter jobdto.JobFilter
	lastGetID      string
}

func (r *recordingService) List(_ context.Context, f jobdto.JobFilter, cursor string, limit int) (jobdto.PageResult, error) {
	r.listCalls++
	r.lastListFilter = f
	r.lastListCursor = cursor
	r.lastListLimit = limit
	return jobdto.PageResult{NextCursor: "delegated"}, nil
}
func (r *recordingService) Get(_ context.Context, id string) (*jobdto.JobView, error) {
	r.getCalls++
	r.lastGetID = id
	return nil, nil
}
func (r *recordingService) SetSaved(_ context.Context, id string, saved bool) error {
	r.setSavedCalls++
	r.lastSavedID = id
	r.lastSavedBool = saved
	return nil
}
func (r *recordingService) SetStatus(_ context.Context, id, status string) error {
	r.setStatusCalls++
	r.lastID = id
	r.lastStatus = status
	return nil
}
func (r *recordingService) SetNotes(_ context.Context, id, notes string) error {
	r.setNotesCalls++
	r.lastID = id
	r.lastNotes = notes
	return nil
}
func (r *recordingService) GetCounts(_ context.Context) (jobdto.Counts, error) {
	r.countsCalls++
	return jobdto.Counts{Saved: 7}, nil
}
func (r *recordingService) GetCompanies(_ context.Context) ([]string, error) {
	r.companiesCalls++
	return []string{"x"}, nil
}

var _ jobsvc.JobService = (*recordingService)(nil)

// passThroughService embeds BaseService verbatim — any method that isn't
// overridden delegates to next via the BaseService. Used to verify each
// BaseService method correctly forwards arguments.
type passThroughService struct {
	jobsvc.BaseService
}

// TestBaseService_PanicsOnNilNext locks the wiring-error contract: the
// constructor panics rather than letting a silent NPE surface later.
func TestBaseService_PanicsOnNilNext(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil next")
		}
	}()
	_ = jobsvc.NewBaseService(nil)
}

// TestBaseService_DelegatesEveryMethod exercises every pass-through method
// in BaseService. Each call must (a) reach the recording next, (b) carry
// the exact argument it was given, (c) return whatever the recording layer
// returned.
func TestBaseService_DelegatesEveryMethod(t *testing.T) {
	next := &recordingService{}
	pt := &passThroughService{BaseService: jobsvc.NewBaseService(next)}
	ctx := context.Background()

	// List
	page, err := pt.List(ctx, jobdto.JobFilter{Status: "saved", Search: "go"}, "tok", 25)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if page.NextCursor != "delegated" {
		t.Errorf("NextCursor = %q, want \"delegated\"", page.NextCursor)
	}
	if next.listCalls != 1 {
		t.Errorf("listCalls = %d, want 1", next.listCalls)
	}
	if next.lastListCursor != "tok" || next.lastListLimit != 25 || next.lastListFilter.Status != "saved" {
		t.Errorf("List forwarded args wrong: cursor=%q limit=%d filter=%+v",
			next.lastListCursor, next.lastListLimit, next.lastListFilter)
	}

	// Get
	if _, err := pt.Get(ctx, "j-get"); err != nil {
		t.Fatalf("Get err = %v", err)
	}
	if next.lastGetID != "j-get" {
		t.Errorf("Get id = %q, want j-get", next.lastGetID)
	}

	// SetSaved
	if err := pt.SetSaved(ctx, "j-s", true); err != nil {
		t.Fatalf("SetSaved err = %v", err)
	}
	if next.lastSavedID != "j-s" || !next.lastSavedBool {
		t.Errorf("SetSaved forwarded wrong: id=%q saved=%v", next.lastSavedID, next.lastSavedBool)
	}

	// SetStatus
	if err := pt.SetStatus(ctx, "j-st", "applied"); err != nil {
		t.Fatalf("SetStatus err = %v", err)
	}
	if next.lastID != "j-st" || next.lastStatus != "applied" {
		t.Errorf("SetStatus forwarded wrong: id=%q status=%q", next.lastID, next.lastStatus)
	}

	// SetNotes
	if err := pt.SetNotes(ctx, "j-n", "hello"); err != nil {
		t.Fatalf("SetNotes err = %v", err)
	}
	if next.lastNotes != "hello" {
		t.Errorf("SetNotes forwarded wrong notes: %q", next.lastNotes)
	}

	// GetCounts
	c, err := pt.GetCounts(ctx)
	if err != nil {
		t.Fatalf("GetCounts err = %v", err)
	}
	if c.Saved != 7 {
		t.Errorf("counts.Saved = %d, want 7", c.Saved)
	}

	// GetCompanies
	names, err := pt.GetCompanies(ctx)
	if err != nil {
		t.Fatalf("GetCompanies err = %v", err)
	}
	if len(names) != 1 || names[0] != "x" {
		t.Errorf("companies = %v, want [x]", names)
	}
}
