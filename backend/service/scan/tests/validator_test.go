package scan

import (
	"context"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// noopService is a terminal Service that succeeds for every call.
// Used to back the validationService in unit tests.
type noopService struct{}

func (n *noopService) TriggerScan(_ context.Context, _ string) error { return nil }
func (n *noopService) TriggerScanAll(_ context.Context) (int, error) { return 0, nil }
func (n *noopService) GetStats(_ context.Context, _ string) (*dto.ScanStatsView, error) {
	return &dto.ScanStatsView{}, nil
}
func (n *noopService) ListQueue(_ context.Context, _ dto.QueueFilter) ([]dto.QueueEntryView, error) {
	return nil, nil
}
func (n *noopService) RemoveFromQueue(_ context.Context, _ string) error   { return nil }
func (n *noopService) EnqueueDue(_ context.Context, _ int) (int, error)    { return 0, nil }
func (n *noopService) ResetStuck(_ context.Context, _ time.Duration) error { return nil }

var _ svc.Service = (*noopService)(nil)

func TestValidateTriggerScan(t *testing.T) {
	v := svc.NewValidator(&noopService{})

	if err := v.TriggerScan(context.Background(), ""); err == nil {
		t.Error("expected error for empty companyID")
	}
	if err := v.TriggerScan(context.Background(), "stripe"); err != nil {
		t.Errorf("unexpected error for valid companyID: %v", err)
	}
}

func TestValidateListQueue(t *testing.T) {
	v := svc.NewValidator(&noopService{})

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
			_, err := v.ListQueue(context.Background(), tc.filter)
			if (err != nil) != tc.wantErr {
				t.Errorf("ListQueue() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
