package job

import (
	"testing"

	"github.com/trushilpatel/jobradar/api/request"
	"github.com/trushilpatel/jobradar/model"
)

func TestValidateList(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		filter  JobFilter
		p       request.Pagination
		wantErr bool
	}{
		{name: "valid new status", filter: JobFilter{Status: model.JobStatusNew}, p: request.Pagination{Page: 1, Limit: 20}},
		{name: "valid saved status", filter: JobFilter{Status: model.JobStatusSaved}, p: request.Pagination{Page: 1, Limit: 20}},
		{name: "valid applied status", filter: JobFilter{Status: model.JobStatusApplied}, p: request.Pagination{Page: 1, Limit: 20}},
		{name: "valid interviewing status", filter: JobFilter{Status: model.JobStatusInterviewing}, p: request.Pagination{Page: 1, Limit: 20}},
		{name: "valid offer status", filter: JobFilter{Status: model.JobStatusOffer}, p: request.Pagination{Page: 1, Limit: 20}},
		{name: "invalid status", filter: JobFilter{Status: "dismissed"}, p: request.Pagination{Page: 1, Limit: 20}, wantErr: true},
		{name: "page 0 invalid", filter: JobFilter{}, p: request.Pagination{Page: 0, Limit: 20}, wantErr: true},
		{name: "limit 0 invalid", filter: JobFilter{}, p: request.Pagination{Page: 1, Limit: 0}, wantErr: true},
		{name: "limit 101 invalid", filter: JobFilter{}, p: request.Pagination{Page: 1, Limit: 101}, wantErr: true},
		{name: "limit 100 valid", filter: JobFilter{}, p: request.Pagination{Page: 1, Limit: 100}},
		{name: "empty status valid", filter: JobFilter{}, p: request.Pagination{Page: 1, Limit: 20}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := v.ValidateList(tc.filter, tc.p)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateList() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateDismiss(t *testing.T) {
	v := NewValidator()

	if err := v.ValidateDismiss(DismissInput{Reason: model.DiscardReasonDismissed}); err != nil {
		t.Errorf("unexpected error for valid reason: %v", err)
	}
	if err := v.ValidateDismiss(DismissInput{Reason: model.DiscardReasonNotRelevant}); err != nil {
		t.Errorf("unexpected error for valid reason: %v", err)
	}
	if err := v.ValidateDismiss(DismissInput{Reason: "bad_reason"}); err == nil {
		t.Error("expected error for invalid reason")
	}
	if err := v.ValidateDismiss(DismissInput{Reason: ""}); err == nil {
		t.Error("expected error for empty reason")
	}
}

func TestValidateSetStatus(t *testing.T) {
	v := NewValidator()

	if err := v.ValidateSetStatus(model.JobStatusNew); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.ValidateSetStatus(model.JobStatusSaved); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.ValidateSetStatus(model.JobStatusApplied); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.ValidateSetStatus(model.JobStatusInterviewing); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.ValidateSetStatus(model.JobStatusOffer); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := v.ValidateSetStatus(model.JobStatusDismissed); err == nil {
		t.Error("expected error for dismissed status")
	}
	if err := v.ValidateSetStatus(""); err == nil {
		t.Error("expected error for empty status")
	}
}
