package scan

import (
	"testing"

	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

func TestValidateTriggerScan(t *testing.T) {
	v := svc.NewValidator()

	if err := v.ValidateTriggerScan(""); err == nil {
		t.Error("expected error for empty companyID")
	}
	if err := v.ValidateTriggerScan("stripe"); err != nil {
		t.Errorf("unexpected error for valid companyID: %v", err)
	}
}

func TestValidateListQueue(t *testing.T) {
	v := svc.NewValidator()

	tests := []struct {
		name    string
		filter  dto.QueueFilter
		wantErr bool
	}{
		{name: "empty filter", filter: dto.QueueFilter{}},
		{name: "valid status queued", filter: dto.QueueFilter{Status: "queued"}},
		{name: "valid status processing", filter: dto.QueueFilter{Status: "processing"}},
		{name: "valid source manual", filter: dto.QueueFilter{Source: "manual"}},
		{name: "valid source scheduler", filter: dto.QueueFilter{Source: "scheduler"}},
		{name: "invalid status", filter: dto.QueueFilter{Status: "invalid"}, wantErr: true},
		{name: "invalid source", filter: dto.QueueFilter{Source: "unknown"}, wantErr: true},
		{name: "valid both", filter: dto.QueueFilter{Status: "queued", Source: "manual"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := v.ValidateListQueue(tc.filter)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateListQueue() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
