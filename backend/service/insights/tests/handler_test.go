package insights

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/insights/dto"
	svc "github.com/trushilpatel/jobradar/service/insights/services"
)

type fakeInsightsService struct {
	mu                    sync.Mutex
	techCounts            []dto.TechCount
	activity              []dto.CompanyActivityItem
	funnel                []dto.FunnelStep
	location              dto.LocationBreakdown
	sponsorship           []dto.SponsorshipDay
	velocity              []dto.CompanyVelocity
	roleCategories        []dto.RoleCategoryCount
	pipelineCounts        dto.PipelineCounts
	err                   error
	lastSponsorshipFilter dto.InsightsFilter
}

func (f *fakeInsightsService) TechStack(_ context.Context) ([]dto.TechCount, error) {
	return f.techCounts, f.err
}
func (f *fakeInsightsService) CompanyActivity(_ context.Context) ([]dto.CompanyActivityItem, error) {
	return f.activity, f.err
}
func (f *fakeInsightsService) Funnel(_ context.Context) ([]dto.FunnelStep, error) {
	return f.funnel, f.err
}
func (f *fakeInsightsService) Location(_ context.Context) (dto.LocationBreakdown, error) {
	return f.location, f.err
}
func (f *fakeInsightsService) Sponsorship(_ context.Context, filter dto.InsightsFilter) ([]dto.SponsorshipDay, error) {
	f.mu.Lock()
	f.lastSponsorshipFilter = filter
	f.mu.Unlock()
	return f.sponsorship, f.err
}
func (f *fakeInsightsService) Velocity(_ context.Context) ([]dto.CompanyVelocity, error) {
	return f.velocity, f.err
}
func (f *fakeInsightsService) RoleCategories(_ context.Context) ([]dto.RoleCategoryCount, error) {
	return f.roleCategories, f.err
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
	h := svc.NewRestService(&fakeInsightsService{techCounts: []dto.TechCount{}})
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
	h := svc.NewRestService(&fakeInsightsService{err: errors.ErrInternal.WithMessage("db down")})
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
	h := svc.NewRestService(&fakeInsightsService{funnel: []dto.FunnelStep{}})
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/funnel", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestInsightsHandlerSponsorship_OK(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{sponsorship: []dto.SponsorshipDay{}})
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/sponsorship?days=30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestInsightsHandlerSponsorship_ValidationError(t *testing.T) {
	fake := &fakeInsightsService{sponsorship: []dto.SponsorshipDay{}}
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
	h := svc.NewRestService(&fakeInsightsService{velocity: []dto.CompanyVelocity{}})
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/velocity", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestInsightsHandlerRoleCategories_OK(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{roleCategories: []dto.RoleCategoryCount{}})
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/role-categories", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestInsightsHandlerCompanyActivity_OK(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{activity: []dto.CompanyActivityItem{}})
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/company-activity", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"data"`) {
		t.Errorf("body missing data envelope: %s", w.Body.String())
	}
}

func TestInsightsHandlerCompanyActivity_ServiceError(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{err: errors.ErrInternal.WithMessage("db down")})
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/company-activity", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestInsightsHandlerFunnel_ServiceError(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{err: errors.ErrInternal.WithMessage("db down")})
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/funnel", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestInsightsHandlerLocation_OK(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{
		location: dto.LocationBreakdown{
			Countries: []dto.LocationCount{},
			States:    []dto.StateCount{},
		},
	})
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/location", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"countries"`) {
		t.Errorf("body missing countries: %s", w.Body.String())
	}
}

func TestInsightsHandlerLocation_ServiceError(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{err: errors.ErrInternal.WithMessage("db down")})
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/location", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestInsightsHandlerSponsorship_DefaultDays(t *testing.T) {
	fake := &fakeInsightsService{sponsorship: []dto.SponsorshipDay{}}
	h := svc.NewRestService(fake)
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/sponsorship", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if fake.lastSponsorshipFilter.Days != 30 {
		t.Errorf("filter.Days = %d, want 30 (default when query param absent)", fake.lastSponsorshipFilter.Days)
	}
}

// TestInsightsHandlerSponsorship_InvalidDays asserts a non-numeric days param
// returns 400 — the handler rejects unparseable values explicitly.
func TestInsightsHandlerSponsorship_InvalidDays(t *testing.T) {
	fake := &fakeInsightsService{sponsorship: []dto.SponsorshipDay{}}
	h := svc.NewRestService(fake)
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/sponsorship?days=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400 (non-numeric days rejected)", w.Code)
	}
}

func TestInsightsHandlerSponsorship_CustomDays(t *testing.T) {
	fake := &fakeInsightsService{sponsorship: []dto.SponsorshipDay{}}
	h := svc.NewRestService(fake)
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/sponsorship?days=7", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if fake.lastSponsorshipFilter.Days != 7 {
		t.Errorf("filter.Days = %d, want 7", fake.lastSponsorshipFilter.Days)
	}
}

func TestInsightsHandlerSponsorship_NegativeDays_PassesParser(t *testing.T) {
	fake := &fakeInsightsService{sponsorship: []dto.SponsorshipDay{}}
	h := svc.NewRestService(fake)
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/sponsorship?days=-5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if fake.lastSponsorshipFilter.Days != -5 {
		t.Errorf("filter.Days = %d, want -5", fake.lastSponsorshipFilter.Days)
	}
}

func TestInsightsHandlerSponsorship_NegativeDays_RejectedByValidation(t *testing.T) {
	fake := &fakeInsightsService{sponsorship: []dto.SponsorshipDay{}}
	validation := svc.NewValidationService(fake)
	h := svc.NewRestService(validation)
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/sponsorship?days=-5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"days"`) {
		t.Errorf("body missing days field error: %s", w.Body.String())
	}
}

func TestInsightsHandlerSponsorship_ServiceError(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{err: errors.ErrInternal.WithMessage("db down")})
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/sponsorship?days=30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestInsightsHandlerVelocity_ServiceError(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{err: errors.ErrInternal.WithMessage("db down")})
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/velocity", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestInsightsHandlerRoleCategories_ServiceError(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{err: errors.ErrInternal.WithMessage("db down")})
	r := newInsightsTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/insights/role-categories", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestInsightsRouter_RegisteredRoutes(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{techCounts: []dto.TechCount{}})
	r := newInsightsTestRouter(h)
	for _, path := range []string{
		"/insights/tech-stack",
		"/insights/company-activity",
		"/insights/funnel",
		"/insights/location",
		"/insights/sponsorship",
		"/insights/velocity",
		"/insights/role-categories",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Errorf("route %q not registered (got 404)", path)
		}
	}
}

var _ svc.InsightsService = (*fakeInsightsService)(nil)
