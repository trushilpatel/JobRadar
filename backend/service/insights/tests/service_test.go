package insights

import (
	"context"
	"errors"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
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

func (f *fakeInsightsRepository) TechStack(_ context.Context) (interface{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.techCounts, nil
}
func (f *fakeInsightsRepository) CompanyActivity(_ context.Context) (interface{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.activity, nil
}
func (f *fakeInsightsRepository) Funnel(_ context.Context) (interface{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.funnel, nil
}
func (f *fakeInsightsRepository) Location(_ context.Context) (interface{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	return map[string]interface{}{"countries": f.countries, "states": f.states}, nil
}
func (f *fakeInsightsRepository) Sponsorship(_ context.Context, filter dto.InsightsFilter) (interface{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sponsorship, nil
}
func (f *fakeInsightsRepository) Velocity(_ context.Context) (interface{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.velocity, nil
}
func (f *fakeInsightsRepository) RoleCategories(_ context.Context) (interface{}, error) {
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
	// RepositoryService wraps DB errors into apperr.ServiceError before returning.
	// Simulate that by having the fake return a wrapped ServiceError directly.
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
	// days=0 — ValidationService would reject it, but ImplService passes through to repo
	data, err := service.Sponsorship(context.Background(), dto.InsightsFilter{Days: 0})
	if err != nil {
		t.Fatalf("Sponsorship() error = %v", err)
	}
	if data == nil {
		t.Error("expected non-nil data")
	}
}

// Ensure test double implements the InsightsService interface.
var _ svc.InsightsService = (*fakeInsightsRepository)(nil)
