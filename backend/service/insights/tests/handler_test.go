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

// fakeInsightsService is a test double for InsightsService.
type fakeInsightsService struct {
	mu             sync.Mutex
	data           interface{}
	pipelineCounts dto.PipelineCounts
	err            error
	// Capture the most recent Sponsorship filter so handler tests can assert
	// the days query-param parsing behavior end-to-end.
	lastSponsorshipFilter dto.InsightsFilter
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
func (f *fakeInsightsService) Sponsorship(_ context.Context, filter dto.InsightsFilter) (interface{}, error) {
	f.mu.Lock()
	f.lastSponsorshipFilter = filter
	f.mu.Unlock()
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
		&fakeInsightsService{err: errors.ErrInternal.WithMessage("db down")},
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

// TestInsightsHandlerCompanyActivity_OK ensures the company activity endpoint
// returns 200 with a data envelope on success.
func TestInsightsHandlerCompanyActivity_OK(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{data: []dto.CompanyActivityItem{}})
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
	h := svc.NewRestService(
		&fakeInsightsService{err: errors.ErrInternal.WithMessage("db down")},
	)
	r := newInsightsTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/insights/company-activity", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestInsightsHandlerFunnel_ServiceError(t *testing.T) {
	h := svc.NewRestService(
		&fakeInsightsService{err: errors.ErrInternal.WithMessage("db down")},
	)
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
		data: map[string]interface{}{
			"countries": []dto.LocationCount{},
			"states":    []dto.StateCount{},
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
	h := svc.NewRestService(
		&fakeInsightsService{err: errors.ErrInternal.WithMessage("db down")},
	)
	r := newInsightsTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/insights/location", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

// TestInsightsHandlerSponsorship_DefaultDays asserts the days query-param
// defaults to 30 when omitted entirely. This is the documented contract.
func TestInsightsHandlerSponsorship_DefaultDays(t *testing.T) {
	fake := &fakeInsightsService{data: []dto.SponsorshipDay{}}
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

// TestInsightsHandlerSponsorship_InvalidDays asserts a non-numeric days
// param falls back to the 30-day default rather than producing an error —
// strconv.Atoi failures are silently ignored.
func TestInsightsHandlerSponsorship_InvalidDays(t *testing.T) {
	fake := &fakeInsightsService{data: []dto.SponsorshipDay{}}
	h := svc.NewRestService(fake)
	r := newInsightsTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/insights/sponsorship?days=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if fake.lastSponsorshipFilter.Days != 30 {
		t.Errorf("filter.Days = %d, want 30 (default on parse failure)", fake.lastSponsorshipFilter.Days)
	}
}

// TestInsightsHandlerSponsorship_CustomDays asserts a valid numeric days
// param is forwarded verbatim to the underlying service.
func TestInsightsHandlerSponsorship_CustomDays(t *testing.T) {
	fake := &fakeInsightsService{data: []dto.SponsorshipDay{}}
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

// TestInsightsHandlerSponsorship_NegativeDays asserts negative days passes
// through the parse step (strconv.Atoi accepts them). Validation, when wired,
// catches this; the handler alone does not reject.
func TestInsightsHandlerSponsorship_NegativeDays_PassesParser(t *testing.T) {
	fake := &fakeInsightsService{data: []dto.SponsorshipDay{}}
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

// TestInsightsHandlerSponsorship_NegativeDays_RejectedByValidation wires the
// validation layer in front of the handler and confirms a negative `days`
// query string is rejected with 400.
func TestInsightsHandlerSponsorship_NegativeDays_RejectedByValidation(t *testing.T) {
	fake := &fakeInsightsService{data: []dto.SponsorshipDay{}}
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
	h := svc.NewRestService(
		&fakeInsightsService{err: errors.ErrInternal.WithMessage("db down")},
	)
	r := newInsightsTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/insights/sponsorship?days=30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestInsightsHandlerVelocity_ServiceError(t *testing.T) {
	h := svc.NewRestService(
		&fakeInsightsService{err: errors.ErrInternal.WithMessage("db down")},
	)
	r := newInsightsTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/insights/velocity", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestInsightsHandlerRoleCategories_ServiceError(t *testing.T) {
	h := svc.NewRestService(
		&fakeInsightsService{err: errors.ErrInternal.WithMessage("db down")},
	)
	r := newInsightsTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/insights/role-categories", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

// TestInsightsRouter_RegisteredRoutes asserts every documented endpoint is
// mounted exactly once by NewRestRouter — protects against accidental route
// removal in future refactors.
func TestInsightsRouter_RegisteredRoutes(t *testing.T) {
	h := svc.NewRestService(&fakeInsightsService{data: []dto.TechCount{}})
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

// Ensure test double implements InsightsService.
var _ svc.InsightsService = (*fakeInsightsService)(nil)
