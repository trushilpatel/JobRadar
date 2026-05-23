package insights

import (
	"context"
	"errors"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/insights/common"
	"github.com/trushilpatel/jobradar/service/insights/dto"
	svc "github.com/trushilpatel/jobradar/service/insights/services"
)

// fakeInsightsRepository is a test double for the terminal InsightsService (repository layer).
type fakeInsightsRepository struct {
	techCounts     []dto.TechCount
	activity       []dto.CompanyActivityItem
	funnel         []dto.FunnelStep
	countries      []dto.LocationCount
	states         []dto.StateCount
	sponsorship    []dto.SponsorshipDay
	velocity       []dto.CompanyVelocity
	roleCategories []dto.RoleCategoryCount
	pipelineCounts dto.PipelineCounts
	err            error
}

func (f *fakeInsightsRepository) TechStack(_ context.Context) ([]dto.TechCount, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.techCounts, nil
}
func (f *fakeInsightsRepository) CompanyActivity(_ context.Context) ([]dto.CompanyActivityItem, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.activity, nil
}
func (f *fakeInsightsRepository) Funnel(_ context.Context) ([]dto.FunnelStep, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.funnel, nil
}
func (f *fakeInsightsRepository) Location(_ context.Context) (dto.LocationBreakdown, error) {
	if f.err != nil {
		return dto.LocationBreakdown{}, f.err
	}
	return dto.LocationBreakdown{Countries: f.countries, States: f.states}, nil
}
func (f *fakeInsightsRepository) Sponsorship(_ context.Context, _ dto.InsightsFilter) ([]dto.SponsorshipDay, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sponsorship, nil
}
func (f *fakeInsightsRepository) Velocity(_ context.Context) ([]dto.CompanyVelocity, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.velocity, nil
}
func (f *fakeInsightsRepository) RoleCategories(_ context.Context) ([]dto.RoleCategoryCount, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.roleCategories, nil
}
func (f *fakeInsightsRepository) GetPipelineCounts(_ context.Context) (dto.PipelineCounts, error) {
	return f.pipelineCounts, f.err
}

func TestTechStack_OK(t *testing.T) {
	repo := &fakeInsightsRepository{techCounts: []dto.TechCount{{Tech: "Go", Count: 5}}}
	service := svc.NewImplService(repo)
	data, err := service.TechStack(context.Background())
	if err != nil {
		t.Fatalf("TechStack() error = %v", err)
	}
	if data == nil {
		t.Error("expected non-nil data")
	}
}

func TestTechStack_Error(t *testing.T) {
	repo := &fakeInsightsRepository{err: apperr.ErrInternal.WithMessage("failed to fetch tech stack")}
	service := svc.NewImplService(repo)
	_, err := service.TechStack(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Status != 500 {
		t.Errorf("expected 500 ServiceError, got %v", err)
	}
}

func TestSponsorship_DefaultDays(t *testing.T) {
	repo := &fakeInsightsRepository{sponsorship: []dto.SponsorshipDay{}}
	service := svc.NewImplService(repo)
	data, err := service.Sponsorship(context.Background(), dto.InsightsFilter{Days: 0})
	if err != nil {
		t.Fatalf("Sponsorship() error = %v", err)
	}
	if data == nil {
		t.Error("expected non-nil data")
	}
}

func TestCompanyActivity_OK(t *testing.T) {
	repo := &fakeInsightsRepository{activity: []dto.CompanyActivityItem{{CompanyName: "Acme", Week: "2025-01-01", Count: 3}}}
	service := svc.NewImplService(repo)
	rows, err := service.CompanyActivity(context.Background())
	if err != nil {
		t.Fatalf("CompanyActivity() error = %v", err)
	}
	if len(rows) != 1 || rows[0].CompanyName != "Acme" {
		t.Errorf("unexpected rows: %+v", rows)
	}
}

func TestCompanyActivity_Error(t *testing.T) {
	repo := &fakeInsightsRepository{err: apperr.ErrInternal.WithMessage("db down")}
	service := svc.NewImplService(repo)
	_, err := service.CompanyActivity(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFunnel_OK(t *testing.T) {
	repo := &fakeInsightsRepository{funnel: []dto.FunnelStep{{Stage: "enriched", Count: 10}}}
	service := svc.NewImplService(repo)
	steps, err := service.Funnel(context.Background())
	if err != nil {
		t.Fatalf("Funnel() error = %v", err)
	}
	if len(steps) == 0 {
		t.Error("expected non-empty steps")
	}
}

func TestFunnel_Error(t *testing.T) {
	repo := &fakeInsightsRepository{err: apperr.ErrInternal.WithMessage("db down")}
	service := svc.NewImplService(repo)
	_, err := service.Funnel(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLocation_OK(t *testing.T) {
	repo := &fakeInsightsRepository{
		countries: []dto.LocationCount{{Country: "USA", Count: 5}},
		states:    []dto.StateCount{{State: "CA", Count: 3}},
	}
	service := svc.NewImplService(repo)
	breakdown, err := service.Location(context.Background())
	if err != nil {
		t.Fatalf("Location() error = %v", err)
	}
	if len(breakdown.Countries) == 0 {
		t.Error("missing countries")
	}
	if len(breakdown.States) == 0 {
		t.Error("missing states")
	}
}

func TestRoleCategories_OK(t *testing.T) {
	repo := &fakeInsightsRepository{roleCategories: []dto.RoleCategoryCount{{Category: "engineering", Count: 50}}}
	service := svc.NewImplService(repo)
	rows, err := service.RoleCategories(context.Background())
	if err != nil {
		t.Fatalf("RoleCategories() error = %v", err)
	}
	if len(rows) == 0 {
		t.Error("expected non-empty rows")
	}
}

func TestRoleCategories_Error(t *testing.T) {
	repo := &fakeInsightsRepository{err: apperr.ErrInternal.WithMessage("db down")}
	service := svc.NewImplService(repo)
	_, err := service.RoleCategories(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetPipelineCounts_OK(t *testing.T) {
	repo := &fakeInsightsRepository{pipelineCounts: dto.PipelineCounts{Pending: 7, Errors: 2}}
	service := svc.NewImplService(repo)
	counts, err := service.GetPipelineCounts(context.Background())
	if err != nil {
		t.Fatalf("GetPipelineCounts() error = %v", err)
	}
	if counts.Pending != 7 || counts.Errors != 2 {
		t.Errorf("got %+v, want {Pending:7 Errors:2}", counts)
	}
}

func TestGetPipelineCounts_Error(t *testing.T) {
	repo := &fakeInsightsRepository{err: apperr.ErrInternal.WithMessage("db down")}
	service := svc.NewImplService(repo)
	_, err := service.GetPipelineCounts(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Status != 500 {
		t.Errorf("expected 500 ServiceError, got %v", err)
	}
}

func TestSponsorship_RepoError(t *testing.T) {
	repo := &fakeInsightsRepository{err: apperr.ErrInternal.WithMessage("db down")}
	service := svc.NewImplService(repo)
	_, err := service.Sponsorship(context.Background(), dto.InsightsFilter{Days: 30})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestVelocity_EnrichesRows ensures the impl layer rewrites Multiplier and
// VelocityTag on every row returned by the repository.
func TestVelocity_EnrichesRows(t *testing.T) {
	repo := &fakeInsightsRepository{
		velocity: []dto.CompanyVelocity{
			{CompanyID: "c1", CompanyName: "Spiker", WeekCurrent: 30, Week1: 10, Week2: 10, Week3: 10},
			{CompanyID: "c2", CompanyName: "Riser", WeekCurrent: 13, Week1: 10, Week2: 10, Week3: 10},
			{CompanyID: "c3", CompanyName: "Calm", WeekCurrent: 5, Week1: 10, Week2: 10, Week3: 10},
			{CompanyID: "c4", CompanyName: "Quiet", WeekCurrent: 0, Week1: 0, Week2: 0, Week3: 0},
			{CompanyID: "c5", CompanyName: "FreshSpike", WeekCurrent: 5, Week1: 0, Week2: 0, Week3: 0},
		},
	}
	service := svc.NewImplService(repo)
	rows, err := service.Velocity(context.Background())
	if err != nil {
		t.Fatalf("Velocity() error = %v", err)
	}
	if len(rows) != 5 {
		t.Fatalf("want 5 rows, got %d", len(rows))
	}
	wantTags := []string{
		common.VelocitySpike,
		common.VelocityRising,
		common.VelocityNormal,
		common.VelocityNormal,
		common.VelocitySpike,
	}
	wantMults := []float64{3.0, 1.3, 0.5, 0.0, 3.0}
	for i, r := range rows {
		if r.VelocityTag != wantTags[i] {
			t.Errorf("row %d (%s): tag = %q, want %q", i, r.CompanyName, r.VelocityTag, wantTags[i])
		}
		if r.Multiplier != wantMults[i] {
			t.Errorf("row %d (%s): multiplier = %v, want %v", i, r.CompanyName, r.Multiplier, wantMults[i])
		}
	}
}

func TestVelocity_RepoError(t *testing.T) {
	repo := &fakeInsightsRepository{err: apperr.ErrInternal.WithMessage("db down")}
	service := svc.NewImplService(repo)
	_, err := service.Velocity(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVelocity_EmptyRows(t *testing.T) {
	repo := &fakeInsightsRepository{velocity: []dto.CompanyVelocity{}}
	service := svc.NewImplService(repo)
	rows, err := service.Velocity(context.Background())
	if err != nil {
		t.Fatalf("Velocity() error = %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected empty slice, got %d rows", len(rows))
	}
}

func TestNewBaseService_PanicsOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on NewBaseService(nil)")
		}
	}()
	_ = svc.NewImplService(nil)
}

func TestNewValidationService_PanicsOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on NewValidationService(nil)")
		}
	}()
	_ = svc.NewValidationService(nil)
}

func TestBaseService_DelegatesAll(t *testing.T) {
	spy := &callSpy{}
	service := svc.NewValidationService(spy)
	ctx := context.Background()

	if _, err := service.TechStack(ctx); err != nil {
		t.Errorf("TechStack: %v", err)
	}
	if _, err := service.CompanyActivity(ctx); err != nil {
		t.Errorf("CompanyActivity: %v", err)
	}
	if _, err := service.Funnel(ctx); err != nil {
		t.Errorf("Funnel: %v", err)
	}
	if _, err := service.Location(ctx); err != nil {
		t.Errorf("Location: %v", err)
	}
	if _, err := service.Sponsorship(ctx, dto.InsightsFilter{Days: 30}); err != nil {
		t.Errorf("Sponsorship: %v", err)
	}
	if _, err := service.Velocity(ctx); err != nil {
		t.Errorf("Velocity: %v", err)
	}
	if _, err := service.RoleCategories(ctx); err != nil {
		t.Errorf("RoleCategories: %v", err)
	}
	if _, err := service.GetPipelineCounts(ctx); err != nil {
		t.Errorf("GetPipelineCounts: %v", err)
	}

	want := map[string]int{
		"TechStack":         1,
		"CompanyActivity":   1,
		"Funnel":            1,
		"Location":          1,
		"Sponsorship":       1,
		"Velocity":          1,
		"RoleCategories":    1,
		"GetPipelineCounts": 1,
	}
	for k, v := range want {
		if spy.calls[k] != v {
			t.Errorf("spy %q calls = %d, want %d", k, spy.calls[k], v)
		}
	}
}

type callSpy struct {
	calls map[string]int
}

func (s *callSpy) bump(name string) {
	if s.calls == nil {
		s.calls = map[string]int{}
	}
	s.calls[name]++
}

func (s *callSpy) TechStack(_ context.Context) ([]dto.TechCount, error) {
	s.bump("TechStack")
	return nil, nil
}
func (s *callSpy) CompanyActivity(_ context.Context) ([]dto.CompanyActivityItem, error) {
	s.bump("CompanyActivity")
	return nil, nil
}
func (s *callSpy) Funnel(_ context.Context) ([]dto.FunnelStep, error) {
	s.bump("Funnel")
	return nil, nil
}
func (s *callSpy) Location(_ context.Context) (dto.LocationBreakdown, error) {
	s.bump("Location")
	return dto.LocationBreakdown{}, nil
}
func (s *callSpy) Sponsorship(_ context.Context, _ dto.InsightsFilter) ([]dto.SponsorshipDay, error) {
	s.bump("Sponsorship")
	return nil, nil
}
func (s *callSpy) Velocity(_ context.Context) ([]dto.CompanyVelocity, error) {
	s.bump("Velocity")
	return nil, nil
}
func (s *callSpy) RoleCategories(_ context.Context) ([]dto.RoleCategoryCount, error) {
	s.bump("RoleCategories")
	return nil, nil
}
func (s *callSpy) GetPipelineCounts(_ context.Context) (dto.PipelineCounts, error) {
	s.bump("GetPipelineCounts")
	return dto.PipelineCounts{}, nil
}

var (
	_ svc.InsightsService = (*callSpy)(nil)
	_ svc.InsightsService = (*fakeInsightsRepository)(nil)
)
