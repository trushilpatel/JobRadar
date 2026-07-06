package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	pipelinedto "github.com/trushilpatel/jobradar/service/pipeline/dto"
	pipelinesvc "github.com/trushilpatel/jobradar/service/pipeline/services"
)

// stubPipelineService is a minimal PipelineService double for handler tests.
type stubPipelineService struct {
	statsResult pipelinedto.PipelineStats
	statsErr    error
	retryResult pipelinedto.TriggerRetryResult
	retryErr    error
}

func (s *stubPipelineService) GetStats(_ context.Context) (pipelinedto.PipelineStats, error) {
	return s.statsResult, s.statsErr
}

func (s *stubPipelineService) TriggerRetry(
	_ context.Context,
	_ pipelinedto.TriggerRetryInput,
) (pipelinedto.TriggerRetryResult, error) {
	return s.retryResult, s.retryErr
}

// newPipelineTestRouter wires a chi router with the pipeline RestRouter registered.
func newPipelineTestRouter(svc pipelinesvc.PipelineService) http.Handler {
	rest := pipelinesvc.NewRestService(svc)
	router := pipelinesvc.NewRestRouter(rest)
	mux := chi.NewMux()
	router.Register(mux)
	return mux
}

// ---------- GET /pipeline/stats ----------

func TestPipelineRest_GetStats_200(t *testing.T) {
	t.Parallel()
	svc := &stubPipelineService{statsResult: pipelinedto.PipelineStats{TotalCompanies: 4, Queued: 2}}
	srv := httptest.NewServer(newPipelineTestRouter(svc))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/pipeline/stats")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Data pipelinedto.PipelineStats `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.TotalCompanies != 4 || body.Data.Queued != 2 {
		t.Errorf("got %+v, want TotalCompanies=4 Queued=2", body.Data)
	}
}

func TestPipelineRest_GetStats_500(t *testing.T) {
	t.Parallel()
	svc := &stubPipelineService{statsErr: domainerrs.ErrInternal.WithMessage("db down")}
	srv := httptest.NewServer(newPipelineTestRouter(svc))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/pipeline/stats")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

// ---------- POST /pipeline/retry ----------

func TestPipelineRest_TriggerRetry_200(t *testing.T) {
	t.Parallel()
	svc := &stubPipelineService{retryResult: pipelinedto.TriggerRetryResult{Drained: 0}}
	srv := httptest.NewServer(newPipelineTestRouter(svc))
	defer srv.Close()

	body := strings.NewReader(`{"worker":"filter","tier":"1m"}`)
	resp, err := http.Post(srv.URL+"/pipeline/retry", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Data pipelinedto.TriggerRetryResult `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Data.Drained != 0 {
		t.Errorf("Drained = %d, want 0", out.Data.Drained)
	}
}

func TestPipelineRest_TriggerRetry_400_MalformedBody(t *testing.T) {
	t.Parallel()
	svc := &stubPipelineService{}
	srv := httptest.NewServer(newPipelineTestRouter(svc))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/pipeline/retry", "application/json", strings.NewReader("not json"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPipelineRest_TriggerRetry_400_ValidationError(t *testing.T) {
	t.Parallel()
	svc := &stubPipelineService{retryErr: domainerrs.ErrBadRequest.WithExternal("unknown worker")}
	srv := httptest.NewServer(newPipelineTestRouter(svc))
	defer srv.Close()

	body := strings.NewReader(`{"worker":"bogus","tier":"1m"}`)
	resp, err := http.Post(srv.URL+"/pipeline/retry", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPipelineRest_TriggerRetry_500(t *testing.T) {
	t.Parallel()
	svc := &stubPipelineService{retryErr: domainerrs.ErrInternal.WithMessage("boom")}
	srv := httptest.NewServer(newPipelineTestRouter(svc))
	defer srv.Close()

	body := strings.NewReader(`{"worker":"filter","tier":"1m"}`)
	resp, err := http.Post(srv.URL+"/pipeline/retry", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

// TestPipelineRestService_NilNextPanics verifies that constructing a
// RestService with a nil next panics — guards against misconfigured wire setup.
func TestPipelineRestService_NilNextPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil next")
		}
	}()
	_ = pipelinesvc.NewRestService(nil)
}
