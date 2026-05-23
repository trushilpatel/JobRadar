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
			TotalCompanies:   10,
			ActiveCompanies:  8,
			Queued:           2,
			Crawling:         1,
			NewJobs24h:       5,
			Errors24h:        0,
			PipelinePending:  12,
			PipelineFiltered: 3,
		}},
	)

	stats, err := svc.GetPipelineStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalCompanies != 10 {
		t.Errorf("TotalCompanies = %d, want 10", stats.TotalCompanies)
	}
	// AwaitingFilter comes from DB (pipeline_pending): reason='fetch' not yet in jobs.
	if stats.AwaitingFilter != 12 {
		t.Errorf("AwaitingFilter = %d, want 12 (pipeline_pending from DB)", stats.AwaitingFilter)
	}
	// AwaitingEnrichment comes from DB (pipeline_filtered): reason='filtered' not yet in jobs.
	if stats.AwaitingEnrichment != 3 {
		t.Errorf("AwaitingEnrichment = %d, want 3 (pipeline_filtered from DB)", stats.AwaitingEnrichment)
	}
	// NATS BackOff handles retries — retry tier counts are always zero.
	if stats.FilterRetries.Tier1m != 0 {
		t.Errorf("FilterRetries.Tier1m = %d, want 0 (NATS, no lag)", stats.FilterRetries.Tier1m)
	}
	if stats.EnricherRetries.Tier1m != 0 {
		t.Errorf("EnricherRetries.Tier1m = %d, want 0 (NATS, no lag)", stats.EnricherRetries.Tier1m)
	}
	if stats.NewJobs24h != 5 {
		t.Errorf("NewJobs24h = %d, want 5", stats.NewJobs24h)
	}
}

func TestPipelineService_GetPipelineStats_StoreError(t *testing.T) {
	svc := services.NewPipelineService(
		&fakePipelineStore{err: errors.New("db down")},
	)

	_, err := svc.GetPipelineStats(context.Background())
	if err == nil {
		t.Fatal("expected error from store failure, got nil")
	}
}

func TestPipelineService_TriggerRetry_OK(t *testing.T) {
	svc := services.NewPipelineService(&fakePipelineStore{})

	n, err := svc.TriggerRetry(context.Background(), "filter", "1m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// NATS redelivers automatically — manual drain always returns 0.
	if n != 0 {
		t.Errorf("drained = %d, want 0 (NATS, no drain needed)", n)
	}
}

func TestPipelineService_TriggerRetry_UnknownWorker(t *testing.T) {
	svc := services.NewPipelineService(&fakePipelineStore{})

	_, err := svc.TriggerRetry(context.Background(), "unknown", "1m")
	if err == nil {
		t.Fatal("expected error for unknown worker, got nil")
	}
}

func TestPipelineService_TriggerRetry_UnknownTier(t *testing.T) {
	svc := services.NewPipelineService(&fakePipelineStore{})

	_, err := svc.TriggerRetry(context.Background(), "filter", "99m")
	if err == nil {
		t.Fatal("expected error for unknown tier, got nil")
	}
}

func TestPipelineService_TriggerRetry_AllValidCombinations(t *testing.T) {
	svc := services.NewPipelineService(&fakePipelineStore{})
	workers := []string{"filter", "enricher"}
	tiers := []string{"1m", "10m", "1h", "dlq"}

	for _, w := range workers {
		for _, tier := range tiers {
			n, err := svc.TriggerRetry(context.Background(), w, tier)
			if err != nil {
				t.Errorf("TriggerRetry(%q, %q): unexpected error: %v", w, tier, err)
			}
			if n != 0 {
				t.Errorf("TriggerRetry(%q, %q): got %d, want 0", w, tier, n)
			}
		}
	}
}
