package tests

import (
	"context"
	"errors"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	pipelinedto "github.com/trushilpatel/jobradar/service/pipeline/dto"
	pipelinesvc "github.com/trushilpatel/jobradar/service/pipeline/services"
)

// ---------- GetStats — no validation, pass-through ----------

func TestPipelineValidation_GetStats_PassThrough(t *testing.T) {
	t.Parallel()
	want := pipelinedto.PipelineStats{TotalCompanies: 3}
	next := &recordingPipelineNext{statsResult: want}
	svc := pipelinesvc.NewValidationService(next)

	got, err := svc.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if !next.statsCalled {
		t.Error("next.GetStats not invoked")
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// ---------- TriggerRetry — validation ----------

func TestPipelineValidation_TriggerRetry_UnknownWorker(t *testing.T) {
	t.Parallel()
	next := &recordingPipelineNext{}
	svc := pipelinesvc.NewValidationService(next)

	_, err := svc.TriggerRetry(context.Background(), pipelinedto.TriggerRetryInput{Worker: "analyzer", Tier: "1m"})
	if err == nil {
		t.Fatal("expected error for unknown worker")
	}
	var se *domainerrs.ServiceError
	if !errors.As(err, &se) || !errors.Is(se, domainerrs.ErrBadRequest) {
		t.Errorf("want ErrBadRequest, got %v", err)
	}
	if next.retryCalled {
		t.Error("next.TriggerRetry must not be called on invalid worker")
	}
}

func TestPipelineValidation_TriggerRetry_UnknownTier(t *testing.T) {
	t.Parallel()
	next := &recordingPipelineNext{}
	svc := pipelinesvc.NewValidationService(next)

	_, err := svc.TriggerRetry(context.Background(), pipelinedto.TriggerRetryInput{Worker: "filter", Tier: "5m"})
	if err == nil {
		t.Fatal("expected error for unknown tier")
	}
	var se *domainerrs.ServiceError
	if !errors.As(err, &se) || !errors.Is(se, domainerrs.ErrBadRequest) {
		t.Errorf("want ErrBadRequest, got %v", err)
	}
	if next.retryCalled {
		t.Error("next.TriggerRetry must not be called on invalid tier")
	}
}

func TestPipelineValidation_TriggerRetry_AllValidCombinations(t *testing.T) {
	t.Parallel()
	workers := []string{"filter", "enricher"}
	tiers := []string{"1m", "10m", "1h", "dlq"}
	for _, w := range workers {
		for _, tier := range tiers {
			w, tier := w, tier
			t.Run(w+"/"+tier, func(t *testing.T) {
				t.Parallel()
				next := &recordingPipelineNext{retryResult: pipelinedto.TriggerRetryResult{Drained: 0}}
				svc := pipelinesvc.NewValidationService(next)

				got, err := svc.TriggerRetry(context.Background(), pipelinedto.TriggerRetryInput{Worker: w, Tier: tier})
				if err != nil {
					t.Fatalf("TriggerRetry(%s/%s): %v", w, tier, err)
				}
				if !next.retryCalled {
					t.Error("next.TriggerRetry not invoked")
				}
				if got.Drained != 0 {
					t.Errorf("Drained = %d, want 0", got.Drained)
				}
			})
		}
	}
}
