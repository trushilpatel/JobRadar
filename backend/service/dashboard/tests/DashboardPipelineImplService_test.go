package dashboard_test

import (
	"context"
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/service/dashboard/dto"
	"github.com/trushilpatel/jobradar/service/dashboard/services"
)

// Ensure test doubles implement interfaces.
var _ services.PipelineStore = (*fakePipelineStore)(nil)

// fakeLagReader is a test double for kafka.LagReader.
// lags maps groupID → lag value so each Kafka call can return a distinct count.
type fakeLagReader struct {
	lags map[string]int64
	err  error
}

func (f *fakeLagReader) GetLag(_ context.Context, groupID, _ string) (int64, error) {
	return f.lags[groupID], f.err
}

// fakePipelineStore is a test double for PipelineStore.
type fakePipelineStore struct {
	stats dto.PipelineDBStats
	err   error
}

func (f *fakePipelineStore) GetPipelineDBStats(_ context.Context) (dto.PipelineDBStats, error) {
	return f.stats, f.err
}

func TestPipelineService_GetPipelineStats_OK(t *testing.T) {
	svc := services.NewPipelineService(
		&fakePipelineStore{stats: dto.PipelineDBStats{
			TotalCompanies:  10,
			ActiveCompanies: 8,
			Queued:          2,
			Crawling:        1,
			NewJobs24h:      5,
			Errors24h:       0,
			PipelinePending: 12,
		}},
		&fakeLagReader{lags: map[string]int64{
			"job-post-analyzer": 3,
		}},
	)

	stats, err := svc.GetPipelineStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalCompanies != 10 {
		t.Errorf("TotalCompanies = %d, want 10", stats.TotalCompanies)
	}
	// AwaitingFilter comes from DB (pipeline_pending), not Kafka lag.
	if stats.AwaitingFilter != 12 {
		t.Errorf("AwaitingFilter = %d, want 12 (pipeline_pending from DB)", stats.AwaitingFilter)
	}
	if stats.AwaitingEnrichment != 3 {
		t.Errorf("AwaitingEnrichment = %d, want 3 (job-post-analyzer lag)", stats.AwaitingEnrichment)
	}
	if stats.NewJobs24h != 5 {
		t.Errorf("NewJobs24h = %d, want 5", stats.NewJobs24h)
	}
}

func TestPipelineService_GetPipelineStats_KafkaError(t *testing.T) {
	svc := services.NewPipelineService(
		&fakePipelineStore{stats: dto.PipelineDBStats{TotalCompanies: 5}},
		&fakeLagReader{lags: nil, err: errors.New("kafka unavailable")},
	)

	stats, err := svc.GetPipelineStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.AwaitingFilter != 0 {
		t.Errorf("AwaitingFilter = %d, want 0 on kafka error", stats.AwaitingFilter)
	}
	if stats.AwaitingEnrichment != 0 {
		t.Errorf("AwaitingEnrichment = %d, want 0 on kafka error", stats.AwaitingEnrichment)
	}
}

func TestPipelineService_GetPipelineStats_StoreError(t *testing.T) {
	svc := services.NewPipelineService(
		&fakePipelineStore{err: errors.New("db down")},
		&fakeLagReader{},
	)

	_, err := svc.GetPipelineStats(context.Background())
	if err == nil {
		t.Fatal("expected error from store failure, got nil")
	}
}
