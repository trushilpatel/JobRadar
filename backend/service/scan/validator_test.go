package scan

import (
	"testing"
)

func TestValidateTriggerScan(t *testing.T) {
	v := NewValidator()

	if err := v.ValidateTriggerScan(""); err == nil {
		t.Error("expected error for empty companyID")
	}
	if err := v.ValidateTriggerScan("stripe"); err != nil {
		t.Errorf("unexpected error for valid companyID: %v", err)
	}
}

func TestValidateListQueue(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		filter  QueueFilter
		wantErr bool
	}{
		{name: "empty filter", filter: QueueFilter{}},
		{name: "valid status queued", filter: QueueFilter{Status: "queued"}},
		{name: "valid status processing", filter: QueueFilter{Status: "processing"}},
		{name: "valid source manual", filter: QueueFilter{Source: "manual"}},
		{name: "valid source scheduler", filter: QueueFilter{Source: "scheduler"}},
		{name: "invalid status", filter: QueueFilter{Status: "invalid"}, wantErr: true},
		{name: "invalid source", filter: QueueFilter{Source: "unknown"}, wantErr: true},
		{name: "valid both", filter: QueueFilter{Status: "queued", Source: "manual"}},
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
