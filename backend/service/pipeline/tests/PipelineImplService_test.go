package tests

import (
	"context"
	"errors"
	"testing"

	pipelinedto "github.com/trushilpatel/jobradar/service/pipeline/dto"
	pipelinesvc "github.com/trushilpatel/jobradar/service/pipeline/services"
)

// TestPipelineImplService_PassesThroughGetStats verifies ImplService
// delegates GetStats to the underlying chain without modification.
func TestPipelineImplService_PassesThroughGetStats(t *testing.T) {
	t.Parallel()
	want := pipelinedto.PipelineStats{TotalCompanies: 7}
	next := &recordingPipelineNext{statsResult: want}
	svc := pipelinesvc.NewPipelineImplService(next)

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

// TestPipelineImplService_PassesThroughTriggerRetry verifies ImplService
// delegates TriggerRetry.
func TestPipelineImplService_PassesThroughTriggerRetry(t *testing.T) {
	t.Parallel()
	next := &recordingPipelineNext{retryResult: pipelinedto.TriggerRetryResult{Drained: 0}}
	svc := pipelinesvc.NewPipelineImplService(next)

	got, err := svc.TriggerRetry(context.Background(), pipelinedto.TriggerRetryInput{Worker: "enricher", Tier: "dlq"})
	if err != nil {
		t.Fatalf("TriggerRetry: %v", err)
	}
	if !next.retryCalled {
		t.Error("next.TriggerRetry not invoked")
	}
	if got.Drained != 0 {
		t.Errorf("Drained = %d, want 0", got.Drained)
	}
}

// TestPipelineImplService_PropagatesErrors verifies that errors from next pass through.
func TestPipelineImplService_PropagatesErrors(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("next error")
	next := &recordingPipelineNext{statsErr: wantErr, retryErr: wantErr}
	svc := pipelinesvc.NewPipelineImplService(next)
	ctx := context.Background()

	if _, err := svc.GetStats(ctx); !errors.Is(err, wantErr) {
		t.Errorf("GetStats err = %v, want %v", err, wantErr)
	}
	if _, err := svc.TriggerRetry(ctx, pipelinedto.TriggerRetryInput{}); !errors.Is(err, wantErr) {
		t.Errorf("TriggerRetry err = %v, want %v", err, wantErr)
	}
}
