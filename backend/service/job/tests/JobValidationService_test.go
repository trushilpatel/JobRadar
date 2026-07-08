package services_test

import (
	"context"
	"strings"
	"testing"

	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// noopJobService is a JobService that does nothing — used as the terminal
// next in validation tests so we can verify the validator alone.
type noopJobService struct{}

func (noopJobService) List(_ context.Context, _ jobdto.JobFilter, _ string, _ int) (jobdto.PageResult, error) {
	return jobdto.PageResult{}, nil
}
func (noopJobService) Get(_ context.Context, _ string) (*jobdto.JobView, error) { return nil, nil }
func (noopJobService) SetSaved(_ context.Context, _ string, _ bool) error       { return nil }
func (noopJobService) SetStatus(_ context.Context, _, _ string) error           { return nil }
func (noopJobService) SetNotes(_ context.Context, _, _ string) error            { return nil }
func (noopJobService) GetCounts(_ context.Context) (jobdto.Counts, error) {
	return jobdto.Counts{}, nil
}
func (noopJobService) GetCompanies(_ context.Context) ([]string, error) { return nil, nil }

var _ jobsvc.JobService = noopJobService{}

func newValidator() jobsvc.JobService {
	return jobsvc.NewValidationService(noopJobService{})
}

func ptrInt(n int) *int { return &n }

func TestValidate_List(t *testing.T) {
	v := newValidator()
	tests := []struct {
		name    string
		filter  jobdto.JobFilter
		limit   int
		wantErr bool
	}{
		{name: "valid saved status", filter: jobdto.JobFilter{Status: jobcommon.JobStatusSaved}, limit: 20},
		{name: "valid interviewing status", filter: jobdto.JobFilter{Status: jobcommon.JobStatusInterviewing}, limit: 20},
		{name: "valid offer status", filter: jobdto.JobFilter{Status: jobcommon.JobStatusOffer}, limit: 20},
		{name: "valid rejected status", filter: jobdto.JobFilter{Status: jobcommon.JobStatusRejected}, limit: 20},
		{name: "invalid status", filter: jobdto.JobFilter{Status: "pending"}, limit: 20, wantErr: true},
		{name: "dismissed rejected — feature removed", filter: jobdto.JobFilter{Status: jobcommon.JobStatusDismissed}, limit: 20, wantErr: true},
		{name: "negative limit", filter: jobdto.JobFilter{}, limit: -1, wantErr: true},
		{name: "limit 101", filter: jobdto.JobFilter{}, limit: 101, wantErr: true},
		{name: "experience max <= min", filter: jobdto.JobFilter{ExperienceRanges: []jobdto.ExperienceRange{{Min: 5, Max: ptrInt(3)}}}, limit: 20, wantErr: true},
		{name: "search > 200", filter: jobdto.JobFilter{Search: strings.Repeat("a", 201)}, limit: 20, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := v.List(context.Background(), tc.filter, "", tc.limit)
			if (err != nil) != tc.wantErr {
				t.Errorf("List() err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

// TestValidate_List_StatusesFilter covers validation of the combined
// multi-status filter (filter.Statuses) — used by tabs that show multiple
// workflow states at once (e.g. Tracking). Invalid entries fail validation;
// a valid or empty/nil slice passes without affecting the existing
// filter.Status validation.
func TestValidate_List_StatusesFilter(t *testing.T) {
	v := newValidator()
	tests := []struct {
		name    string
		filter  jobdto.JobFilter
		wantErr bool
	}{
		{name: "nil statuses allowed", filter: jobdto.JobFilter{}},
		{name: "empty statuses allowed", filter: jobdto.JobFilter{Statuses: []string{}}},
		{name: "all valid statuses", filter: jobdto.JobFilter{Statuses: []string{
			jobcommon.JobStatusApplied, jobcommon.JobStatusInterviewing, jobcommon.JobStatusOffer,
		}}},
		{name: "one invalid entry rejected", filter: jobdto.JobFilter{Statuses: []string{
			jobcommon.JobStatusApplied, "bogus",
		}}, wantErr: true},
		{name: "all invalid rejected", filter: jobdto.JobFilter{Statuses: []string{"bogus1", "bogus2"}}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := v.List(context.Background(), tc.filter, "", 20)
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

// TestValidate_SetStatus_AllowsNew — 'new' clears the workflow status
// (UserJobStateRepositoryService.Upsert sets status = NULL, leaving saved_at
// untouched); it is a writable value, not merely a list-filter synonym for
// "no row".
func TestValidate_SetStatus_AllowsNew(t *testing.T) {
	v := newValidator()
	if err := v.SetStatus(context.Background(), "j1", jobcommon.JobStatusNew); err != nil {
		t.Fatalf("'new' must be allowed through to clear workflow status: %v", err)
	}
}

// TestValidate_List_Boundaries exercises the limit and search-length
// boundaries explicitly. limit=0 is allowed (the store clamps to 20); 100 is
// the documented cap; 101+ rejected. search exactly 200 chars is allowed;
// 201 rejected.
func TestValidate_List_Boundaries(t *testing.T) {
	v := newValidator()
	tests := []struct {
		name    string
		filter  jobdto.JobFilter
		limit   int
		wantErr bool
	}{
		{name: "limit 0", limit: 0},
		{name: "limit 100", limit: 100},
		{name: "limit 101", limit: 101, wantErr: true},
		{name: "search 200 chars", filter: jobdto.JobFilter{Search: strings.Repeat("a", 200)}, limit: 20},
		{name: "search 201 chars", filter: jobdto.JobFilter{Search: strings.Repeat("a", 201)}, limit: 20, wantErr: true},
		{name: "empty status allowed for list filter", filter: jobdto.JobFilter{Status: ""}, limit: 20},
		{name: "new status allowed as list filter", filter: jobdto.JobFilter{Status: jobcommon.JobStatusNew}, limit: 20},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := v.List(context.Background(), tc.filter, "", tc.limit)
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

// TestValidate_List_ExperienceRanges covers all the ExperienceRange invariants
// at the boundary so the store never sees a degenerate range.
func TestValidate_List_ExperienceRanges(t *testing.T) {
	v := newValidator()
	tests := []struct {
		name    string
		ranges  []jobdto.ExperienceRange
		wantErr bool
	}{
		{name: "0+ open ended", ranges: []jobdto.ExperienceRange{{Min: 0, Max: nil}}},
		{name: "5+ open ended", ranges: []jobdto.ExperienceRange{{Min: 5, Max: nil}}},
		{name: "0-2 closed", ranges: []jobdto.ExperienceRange{{Min: 0, Max: ptrInt(2)}}},
		{name: "negative min rejected", ranges: []jobdto.ExperienceRange{{Min: -1, Max: nil}}, wantErr: true},
		{name: "max equals min rejected", ranges: []jobdto.ExperienceRange{{Min: 5, Max: ptrInt(5)}}, wantErr: true},
		{name: "two valid bands", ranges: []jobdto.ExperienceRange{{Min: 0, Max: ptrInt(2)}, {Min: 5, Max: nil}}},
		{name: "one valid one invalid → reject", ranges: []jobdto.ExperienceRange{{Min: 0, Max: ptrInt(2)}, {Min: 5, Max: ptrInt(3)}}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := v.List(context.Background(), jobdto.JobFilter{ExperienceRanges: tc.ranges}, "", 20)
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

// TestValidate_SetStatus_AllValidStatuses asserts every writable status
// passes, including 'new' (clears workflow status). 'saved' is NOT writable
// via this path — it is orthogonal and goes through SetSaved instead.
func TestValidate_SetStatus_AllValidStatuses(t *testing.T) {
	v := newValidator()
	allowed := []string{
		jobcommon.JobStatusNew,
		jobcommon.JobStatusApplied,
		jobcommon.JobStatusInterviewing,
		jobcommon.JobStatusOffer,
		jobcommon.JobStatusRejected,
	}
	for _, s := range allowed {
		t.Run(s, func(t *testing.T) {
			if err := v.SetStatus(context.Background(), "j1", s); err != nil {
				t.Errorf("status=%q: %v", s, err)
			}
		})
	}
	rejected := []string{"", "garbage", "NEW", "Saved", jobcommon.JobStatusSaved, jobcommon.JobStatusDismissed}
	for _, s := range rejected {
		t.Run("reject "+s, func(t *testing.T) {
			if err := v.SetStatus(context.Background(), "j1", s); err == nil {
				t.Errorf("status=%q must be rejected", s)
			}
		})
	}
}

// TestValidate_SetStatus_RejectsSaved — 'saved' is not a workflow status;
// the validator rejects it so callers route through SetSaved instead.
func TestValidate_SetStatus_RejectsSaved(t *testing.T) {
	v := newValidator()
	if err := v.SetStatus(context.Background(), "j1", jobcommon.JobStatusSaved); err == nil {
		t.Fatal("'saved' must be rejected — use SetSaved instead")
	}
}

// TestValidate_List_AcceptsSavedFilter — 'saved' IS a valid filter/tab key
// (the impl routes it to GetJobsByStatus → saved_at IS NOT NULL).
func TestValidate_List_AcceptsSavedFilter(t *testing.T) {
	v := newValidator()
	if _, err := v.List(context.Background(), jobdto.JobFilter{Status: jobcommon.JobStatusSaved}, "", 20); err != nil {
		t.Fatalf("saved must be a valid list filter: %v", err)
	}
}
