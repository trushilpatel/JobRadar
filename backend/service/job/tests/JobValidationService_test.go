package services_test

import (
	"context"
	"testing"

	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// noopService is a Service implementation that does nothing — used as the
// terminal next in validation tests so we can verify the validator alone.
type noopService struct{}

func (n *noopService) List(_ context.Context, _ jobdto.JobFilter, _, _ int) ([]jobmodel.Job, int, error) {
	return nil, 0, nil
}
func (n *noopService) Get(_ context.Context, _ string) (*jobmodel.Job, error) { return nil, nil }
func (n *noopService) GetCounts(_ context.Context) (*jobdto.JobCounts, error) {
	return &jobdto.JobCounts{}, nil
}
func (n *noopService) Dismiss(_ context.Context, _ string, _ jobdto.DismissInput) error { return nil }
func (n *noopService) SetSaved(_ context.Context, _ string, _ bool) error               { return nil }
func (n *noopService) SetStatus(_ context.Context, _, _ string) error                   { return nil }
func (n *noopService) SetNotes(_ context.Context, _, _ string) error                    { return nil }
func (n *noopService) MarkApplied(_ context.Context, _ string) error                    { return nil }

var _ jobsvc.Service = (*noopService)(nil)

func newValidator() jobsvc.Service {
	return jobsvc.NewValidator(&noopService{})
}

func TestValidateList(t *testing.T) {
	v := newValidator()

	tests := []struct {
		name    string
		filter  jobdto.JobFilter
		page    int
		limit   int
		wantErr bool
	}{
		{name: "valid new status", filter: jobdto.JobFilter{Status: jobcommon.JobStatusNew}, page: 1, limit: 20},
		{name: "valid saved status", filter: jobdto.JobFilter{Status: jobcommon.JobStatusSaved}, page: 1, limit: 20},
		{name: "valid applied status", filter: jobdto.JobFilter{Status: jobcommon.JobStatusApplied}, page: 1, limit: 20},
		{name: "valid interviewing status", filter: jobdto.JobFilter{Status: jobcommon.JobStatusInterviewing}, page: 1, limit: 20},
		{name: "valid offer status", filter: jobdto.JobFilter{Status: jobcommon.JobStatusOffer}, page: 1, limit: 20},
		{name: "invalid status", filter: jobdto.JobFilter{Status: "dismissed"}, page: 1, limit: 20, wantErr: true},
		{name: "page 0 invalid", filter: jobdto.JobFilter{}, page: 0, limit: 20, wantErr: true},
		{name: "limit 0 invalid", filter: jobdto.JobFilter{}, page: 1, limit: 0, wantErr: true},
		{name: "limit 101 invalid", filter: jobdto.JobFilter{}, page: 1, limit: 101, wantErr: true},
		{name: "limit 100 valid", filter: jobdto.JobFilter{}, page: 1, limit: 100},
		{name: "empty status valid", filter: jobdto.JobFilter{}, page: 1, limit: 20},
		{name: "valid exp 0-2", filter: jobdto.JobFilter{ExperienceRanges: []jobdto.ExperienceRange{{Min: 0, Max: ptrInt(2)}}}, page: 1, limit: 20},
		{name: "valid exp 2-5", filter: jobdto.JobFilter{ExperienceRanges: []jobdto.ExperienceRange{{Min: 2, Max: ptrInt(5)}}}, page: 1, limit: 20},
		{name: "valid exp 5+", filter: jobdto.JobFilter{ExperienceRanges: []jobdto.ExperienceRange{{Min: 5}}}, page: 1, limit: 20},
		{name: "invalid exp max <= min", filter: jobdto.JobFilter{ExperienceRanges: []jobdto.ExperienceRange{{Min: 5, Max: ptrInt(3)}}}, page: 1, limit: 20, wantErr: true},
		{name: "invalid exp negative min", filter: jobdto.JobFilter{ExperienceRanges: []jobdto.ExperienceRange{{Min: -1, Max: ptrInt(2)}}}, page: 1, limit: 20, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := v.List(context.Background(), tc.filter, tc.page, tc.limit)
			if (err != nil) != tc.wantErr {
				t.Errorf("List() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func ptrInt(n int) *int { return &n }

func TestValidateDismiss(t *testing.T) {
	v := newValidator()

	if err := v.Dismiss(context.Background(), "j1", jobdto.DismissInput{Reason: jobcommon.DiscardReasonDismissed}); err != nil {
		t.Errorf("unexpected error for valid reason: %v", err)
	}
	if err := v.Dismiss(context.Background(), "j1", jobdto.DismissInput{Reason: jobcommon.DiscardReasonNotRelevant}); err != nil {
		t.Errorf("unexpected error for valid reason: %v", err)
	}
	if err := v.Dismiss(context.Background(), "j1", jobdto.DismissInput{Reason: "bad_reason"}); err == nil {
		t.Error("expected error for invalid reason")
	}
	if err := v.Dismiss(context.Background(), "j1", jobdto.DismissInput{Reason: ""}); err == nil {
		t.Error("expected error for empty reason")
	}
}

func TestValidateSetStatus(t *testing.T) {
	v := newValidator()

	if err := v.SetStatus(context.Background(), "j1", jobcommon.JobStatusNew); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.SetStatus(context.Background(), "j1", jobcommon.JobStatusSaved); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.SetStatus(context.Background(), "j1", jobcommon.JobStatusApplied); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.SetStatus(context.Background(), "j1", jobcommon.JobStatusInterviewing); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.SetStatus(context.Background(), "j1", jobcommon.JobStatusOffer); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.SetStatus(context.Background(), "j1", jobcommon.JobStatusDismissed); err == nil {
		t.Error("expected error for dismissed status")
	}
	if err := v.SetStatus(context.Background(), "j1", ""); err == nil {
		t.Error("expected error for empty status")
	}
}
