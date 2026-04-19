package insights

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/common/domain"
	"github.com/trushilpatel/jobradar/service/insights/dto"
	svc "github.com/trushilpatel/jobradar/service/insights/services"
)

// fakeInsightsService is a test double for InsightsService.
type fakeInsightsService struct {
	data           interface{}
	pipelineCounts dto.PipelineCounts
	err            error
}

func (f *fakeInsightsService) TechStack(_ context.Context) (interface{}, error) {
	return f.data, f.err
}
func (f *fakeInsightsService) CompanyActivity(_ context.Context) (interface{}, error) {
	return f.data, f.err
}
func (f *fakeInsightsService) Funnel(_ context.Context) (interface{}, error) { return f.data, f.err }
func (f *fakeInsightsService) Location(_ context.Context) (interface{}, error) {
	return f.data, f.err
}
func (f *fakeInsightsService) Sponsorship(_ context.Context, _ dto.InsightsFilter) (interface{}, error) {
	return f.data, f.err
}
func (f *fakeInsightsService) Velocity(_ context.Context) (interface{}, error) {
	return f.data, f.err
}
func (f *fakeInsightsService) RoleCategories(_ context.Context) (interface{}, error) {
	return f.data, f.err
}
func (f *fakeInsightsService) GetPipelineCounts(_ context.Context) (dto.PipelineCounts, error) {
	return f.pipelineCounts, f.err
}

func newInsightsTestRouter(h *svc.RestService) *chi.Mux {
	r := chi.NewRouter()
	router := svc.NewRestRouter(h)
	router.Register(r)
	return r
}

func TestInsightsHandlerTechStack_OK(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{data: []dto.TechCount{}})
	r := newInsightsTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/insights/tech-stack", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"data"`) {
		t.Errorf("body missing data envelope: %s", w.Body.String())
	}
}

func TestInsightsHandlerTechStack_ServiceError(t *testing.T) {
	h := svc.NewRestService(
		&fakeInsightsService{err: domain.ErrInternal.WithMessage("db down")},
	)
	r := newInsightsTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/insights/tech-stack", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"error"`) {
		t.Errorf("body missing error envelope: %s", w.Body.String())
	}
}

func TestInsightsHandlerFunnel_OK(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{data: []dto.FunnelStep{}})
	r := newInsightsTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/insights/funnel", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestInsightsHandlerSponsorship_OK(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{data: []dto.SponsorshipDay{}})
	r := newInsightsTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/insights/sponsorship?days=30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestInsightsHandlerSponsorship_ValidationError(t *testing.T) {
	// Wire validation into the chain: ValidationService wraps the fake
	fake := &fakeInsightsService{data: []dto.SponsorshipDay{}}
	validationSvc := svc.NewValidationService(fake)
	h := svc.NewRestService(validationSvc)
	r := newInsightsTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/insights/sponsorship?days=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestInsightsHandlerVelocity_OK(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{data: []dto.CompanyVelocity{}})
	r := newInsightsTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/insights/velocity", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestInsightsHandlerRoleCategories_OK(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{data: []dto.RoleCategoryCount{}})
	r := newInsightsTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/insights/role-categories", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

// Ensure test double implements InsightsService.
var _ svc.InsightsService = (*fakeInsightsService)(nil)
