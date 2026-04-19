package services_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/common/domain"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// fakeJobSvc is a test double for Service used in REST handler tests.
type fakeJobSvc struct {
	jobs  []jobmodel.Job
	job   *jobmodel.Job
	total int
	err   error
}

func (f *fakeJobSvc) List(_ context.Context, _ jobdto.JobFilter, _, _ int) ([]jobmodel.Job, int, error) {
	return f.jobs, f.total, f.err
}
func (f *fakeJobSvc) Get(_ context.Context, _ string) (*jobmodel.Job, error) {
	return f.job, f.err
}
func (f *fakeJobSvc) GetCounts(_ context.Context) (*jobdto.JobCounts, error) {
	return &jobdto.JobCounts{}, f.err
}
func (f *fakeJobSvc) Dismiss(_ context.Context, _ string, _ jobdto.DismissInput) error {
	return f.err
}
func (f *fakeJobSvc) SetSaved(_ context.Context, _ string, _ bool) error { return f.err }
func (f *fakeJobSvc) SetStatus(_ context.Context, _, _ string) error     { return f.err }
func (f *fakeJobSvc) SetNotes(_ context.Context, _, _ string) error      { return f.err }
func (f *fakeJobSvc) MarkApplied(_ context.Context, _ string) error      { return f.err }

var _ jobsvc.Service = (*fakeJobSvc)(nil)

// newJobTestRouter builds a chi router with all job routes registered via RestRouter.
// The fake is used as the terminal; no validation layer is in the chain so REST
// handler tests exercise parsing and HTTP behaviour in isolation.
func newJobTestRouter(fake jobsvc.Service) *chi.Mux {
	rest := jobsvc.NewRestService(fake)
	router := jobsvc.NewRestRouter(rest)
	r := chi.NewRouter()
	router.Register(r)
	return r
}

func TestJobHandlerList_OK(t *testing.T) {
	r := newJobTestRouter(&fakeJobSvc{jobs: []jobmodel.Job{}, total: 0})

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"data"`) {
		t.Errorf("body missing data envelope: %s", body)
	}
	if !strings.Contains(body, `"total"`) {
		t.Errorf("body missing total: %s", body)
	}
}

func TestJobHandlerList_ServiceError(t *testing.T) {
	r := newJobTestRouter(&fakeJobSvc{err: domain.ErrInternal.WithMessage("db failure")})

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestJobHandlerGet_OK(t *testing.T) {
	r := newJobTestRouter(&fakeJobSvc{job: &jobmodel.Job{ID: "j1"}})

	req := httptest.NewRequest(http.MethodGet, "/jobs/j1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestJobHandlerGet_NotFound(t *testing.T) {
	r := newJobTestRouter(&fakeJobSvc{err: domain.ErrNotFound.WithMessage("job not found")})

	req := httptest.NewRequest(http.MethodGet, "/jobs/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestJobHandlerDismiss_OK(t *testing.T) {
	r := newJobTestRouter(&fakeJobSvc{})

	body, _ := json.Marshal(jobdto.DismissInput{Reason: jobcommon.DiscardReasonDismissed})
	req := httptest.NewRequest(http.MethodDelete, "/jobs/j1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestJobHandlerDismiss_ServiceError(t *testing.T) {
	r := newJobTestRouter(&fakeJobSvc{err: domain.ErrInternal.WithMessage("db failure")})

	body, _ := json.Marshal(jobdto.DismissInput{Reason: jobcommon.DiscardReasonDismissed})
	req := httptest.NewRequest(http.MethodDelete, "/jobs/j1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestJobHandlerSetStatus_OK(t *testing.T) {
	r := newJobTestRouter(&fakeJobSvc{})

	body, _ := json.Marshal(jobdto.SetStatusInput{Status: jobcommon.JobStatusApplied})
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/status", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestJobHandlerSetNotes_OK(t *testing.T) {
	r := newJobTestRouter(&fakeJobSvc{})

	body, _ := json.Marshal(jobdto.SetNotesInput{Notes: "great company"})
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/notes", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestJobHandlerGetCounts_OK(t *testing.T) {
	r := newJobTestRouter(&fakeJobSvc{})

	req := httptest.NewRequest(http.MethodGet, "/jobs/counts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"data"`) {
		t.Errorf("body missing data envelope: %s", body)
	}
}

// capturingService records the filter passed to List for inspection.
type capturingService struct {
	filter jobdto.JobFilter
}

func (c *capturingService) List(_ context.Context, f jobdto.JobFilter, _, _ int) ([]jobmodel.Job, int, error) {
	c.filter = f
	return nil, 0, nil
}
func (c *capturingService) Get(_ context.Context, _ string) (*jobmodel.Job, error) { return nil, nil }
func (c *capturingService) GetCounts(_ context.Context) (*jobdto.JobCounts, error) {
	return &jobdto.JobCounts{}, nil
}
func (c *capturingService) Dismiss(_ context.Context, _ string, _ jobdto.DismissInput) error {
	return nil
}
func (c *capturingService) SetSaved(_ context.Context, _ string, _ bool) error { return nil }
func (c *capturingService) SetStatus(_ context.Context, _, _ string) error     { return nil }
func (c *capturingService) SetNotes(_ context.Context, _, _ string) error      { return nil }
func (c *capturingService) MarkApplied(_ context.Context, _ string) error      { return nil }

var _ jobsvc.Service = (*capturingService)(nil)

func TestJobHandlerList_ExperienceRangesParsed(t *testing.T) {
	capturing := &capturingService{}
	r := newJobTestRouter(capturing)

	req := httptest.NewRequest(http.MethodGet, "/jobs?experience_ranges=0-2&experience_ranges=5%2B", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if len(capturing.filter.ExperienceRanges) != 2 {
		t.Fatalf("expected 2 parsed ranges, got %d", len(capturing.filter.ExperienceRanges))
	}
	r0 := capturing.filter.ExperienceRanges[0]
	if r0.Min != 0 || r0.Max == nil || *r0.Max != 2 {
		t.Errorf("range[0] = {Min:%d Max:%v}, want {0 2}", r0.Min, r0.Max)
	}
	r1 := capturing.filter.ExperienceRanges[1]
	if r1.Min != 5 || r1.Max != nil {
		t.Errorf("range[1] = {Min:%d Max:%v}, want {5 nil}", r1.Min, r1.Max)
	}
}

func TestJobHandlerList_ExperienceRangesInvalidDropped(t *testing.T) {
	capturing := &capturingService{}
	r := newJobTestRouter(capturing)

	req := httptest.NewRequest(http.MethodGet, "/jobs?experience_ranges=bad&experience_ranges=2-5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if len(capturing.filter.ExperienceRanges) != 1 {
		t.Fatalf("expected 1 range (invalid dropped), got %d", len(capturing.filter.ExperienceRanges))
	}
	r0 := capturing.filter.ExperienceRanges[0]
	if r0.Min != 2 || r0.Max == nil || *r0.Max != 5 {
		t.Errorf("range[0] = {Min:%d Max:%v}, want {2 5}", r0.Min, r0.Max)
	}
}
