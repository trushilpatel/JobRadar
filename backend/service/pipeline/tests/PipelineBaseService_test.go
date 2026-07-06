package tests

import (
	"context"
	"errors"
	"testing"

	pipelinedto "github.com/trushilpatel/jobradar/service/pipeline/dto"
	pipelinesvc "github.com/trushilpatel/jobradar/service/pipeline/services"
)

// recordingPipelineNext is a minimal PipelineService double that records
// invocations and returns configured values. Used to verify
// BasePipelineService delegates to next without any transformation.
type recordingPipelineNext struct {
	statsCalled bool
	retryCalled bool

	statsResult pipelinedto.PipelineStats
	statsErr    error
	retryResult pipelinedto.TriggerRetryResult
	retryErr    error
}

func (r *recordingPipelineNext) GetStats(_ context.Context) (pipelinedto.PipelineStats, error) {
	r.statsCalled = true
	return r.statsResult, r.statsErr
}

func (r *recordingPipelineNext) TriggerRetry(
	_ context.Context,
	_ pipelinedto.TriggerRetryInput,
) (pipelinedto.TriggerRetryResult, error) {
	r.retryCalled = true
	return r.retryResult, r.retryErr
}

// pipelineBaseHarness embeds BasePipelineService to exercise the
// pass-through path.
type pipelineBaseHarness struct {
	pipelinesvc.BasePipelineService
}

func newPipelineBaseHarness(next pipelinesvc.PipelineService) *pipelineBaseHarness {
	return &pipelineBaseHarness{BasePipelineService: pipelinesvc.NewBasePipelineService(next)}
}

// TestBasePipelineService_NilNextPanics verifies that passing nil to
// NewBasePipelineService is caught as a programmer error at construction time.
func TestBasePipelineService_NilNextPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil next")
		}
	}()
	_ = pipelinesvc.NewBasePipelineService(nil)
}

// TestBasePipelineService_DelegatesGetStats verifies the pass-through path for GetStats.
func TestBasePipelineService_DelegatesGetStats(t *testing.T) {
	t.Parallel()
	want := pipelinedto.PipelineStats{TotalCompanies: 5}
	next := &recordingPipelineNext{statsResult: want}
	b := newPipelineBaseHarness(next)

	got, err := b.GetStats(context.Background())
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

// TestBasePipelineService_DelegatesTriggerRetry verifies the pass-through
// path for TriggerRetry.
func TestBasePipelineService_DelegatesTriggerRetry(t *testing.T) {
	t.Parallel()
	want := pipelinedto.TriggerRetryResult{Drained: 0}
	next := &recordingPipelineNext{retryResult: want}
	b := newPipelineBaseHarness(next)

	got, err := b.TriggerRetry(context.Background(), pipelinedto.TriggerRetryInput{Worker: "filter", Tier: "1m"})
	if err != nil {
		t.Fatalf("TriggerRetry: %v", err)
	}
	if !next.retryCalled {
		t.Error("next.TriggerRetry not invoked")
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// TestBasePipelineService_PropagatesErrors verifies that errors from next
// are returned unmodified.
func TestBasePipelineService_PropagatesErrors(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("downstream failure")
	next := &recordingPipelineNext{statsErr: wantErr, retryErr: wantErr}
	b := newPipelineBaseHarness(next)
	ctx := context.Background()

	if _, err := b.GetStats(ctx); !errors.Is(err, wantErr) {
		t.Errorf("GetStats err = %v, want %v", err, wantErr)
	}
	if _, err := b.TriggerRetry(ctx, pipelinedto.TriggerRetryInput{}); !errors.Is(err, wantErr) {
		t.Errorf("TriggerRetry err = %v, want %v", err, wantErr)
	}
}
