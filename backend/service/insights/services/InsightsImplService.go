package services

import (
	"context"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/service/insights/dto"
)

type implService struct {
	store Store
}

// NewImplService constructs a Service implementation.
func NewImplService(s Store) Service {
	return &implService{store: s}
}

func (s *implService) TechStack(ctx context.Context) (_ interface{}, err error) {
	ctx, span := tracing.Start(ctx, "insights.service.TechStack")
	defer tracing.End(span, &err)
	data, err := s.store.GetTechStackTrends(ctx, 15)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch tech stack")
	}
	return data, nil
}

func (s *implService) CompanyActivity(ctx context.Context) (_ interface{}, err error) {
	ctx, span := tracing.Start(ctx, "insights.service.CompanyActivity")
	defer tracing.End(span, &err)
	data, err := s.store.GetCompanyActivity(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch company activity")
	}
	return data, nil
}

func (s *implService) Funnel(ctx context.Context) (_ interface{}, err error) {
	ctx, span := tracing.Start(ctx, "insights.service.Funnel")
	defer tracing.End(span, &err)
	data, err := s.store.GetPipelineFunnel(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch funnel")
	}
	return data, nil
}

func (s *implService) Location(ctx context.Context) (_ interface{}, err error) {
	ctx, span := tracing.Start(ctx, "insights.service.Location")
	defer tracing.End(span, &err)
	countries, err := s.store.GetLocationBreakdown(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch location breakdown")
	}
	states, err := s.store.GetUSAStateBreakdown(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch state breakdown")
	}
	return map[string]interface{}{
		"countries": countries,
		"states":    states,
	}, nil
}

func (s *implService) Sponsorship(ctx context.Context, filter dto.InsightsFilter) (_ interface{}, err error) {
	ctx, span := tracing.Start(ctx, "insights.service.Sponsorship")
	defer tracing.End(span, &err)
	days := filter.Days
	if days <= 0 {
		days = 30
	}
	data, err := s.store.GetSponsorshipTrend(ctx, days)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch sponsorship trend")
	}
	return data, nil
}

func (s *implService) Velocity(ctx context.Context) (_ interface{}, err error) {
	ctx, span := tracing.Start(ctx, "insights.service.Velocity")
	defer tracing.End(span, &err)
	data, err := s.store.GetCompanyVelocity(ctx, 20)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch company velocity")
	}
	return data, nil
}

func (s *implService) RoleCategories(ctx context.Context) (_ interface{}, err error) {
	ctx, span := tracing.Start(ctx, "insights.service.RoleCategories")
	defer tracing.End(span, &err)
	data, err := s.store.GetRoleCategoryBreakdown(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch role categories")
	}
	return data, nil
}

func (s *implService) GetPipelineCounts(ctx context.Context) (_ dto.PipelineCounts, err error) {
	ctx, span := tracing.Start(ctx, "insights.service.GetPipelineCounts")
	defer tracing.End(span, &err)
	counts, err := s.store.GetPipelineCounts(ctx)
	if err != nil {
		return dto.PipelineCounts{}, domain.ErrInternal.WithMessage("failed to fetch pipeline counts")
	}
	return counts, nil
}
