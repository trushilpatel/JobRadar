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
func (noopJobService) Dismiss(_ context.Context, _ string, _ string, _ string) error {
	return nil
}
func (noopJobService) BulkDismiss(_ context.Context, _ []string, _ string, _ string) error {
	return nil
}
func (noopJobService) GetCounts(_ context.Context) (jobdto.Counts, error) {
	return jobdto.Counts{}, nil
}

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

func TestValidate_SetStatus_RejectsNew(t *testing.T) {
	v := newValidator()
	if err := v.SetStatus(context.Background(), "j1", jobcommon.JobStatusNew); err == nil {
		t.Fatal("'new' must be rejected — delete the row instead")
	}
}

func TestValidate_Dismiss_BadReason(t *testing.T) {
	v := newValidator()
	if err := v.Dismiss(context.Background(), "j1", "garbage", ""); err == nil {
		t.Fatal("invalid reason must be rejected")
	}
}

func TestValidate_BulkDismiss_Empty(t *testing.T) {
	v := newValidator()
	if err := v.BulkDismiss(context.Background(), nil, jobcommon.DismissalReasonNotRelevant, ""); err == nil {
		t.Fatal("empty ids must be rejected")
	}
}
