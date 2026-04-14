package insights

import "github.com/trushilpatel/jobradar/api/response"

// Service defines all analytics queries for the insights domain.
type Service interface {
	TechStack() (interface{}, error)
	CompanyActivity() (interface{}, error)
	Funnel() (interface{}, error)
	Location() (interface{}, error)
	Sponsorship(filter InsightsFilter) (interface{}, error)
	Velocity() (interface{}, error)
	RoleCategories() (interface{}, error)
	GetPipelineCounts() (PipelineCounts, error)
}

type service struct {
	store Store
}

// NewService constructs a Service implementation.
func NewService(s Store) Service {
	return &service{store: s}
}

func (s *service) TechStack() (interface{}, error) {
	data, err := s.store.GetTechStackTrends(15)
	if err != nil {
		return nil, response.ErrInternal.WithMessage("failed to fetch tech stack")
	}
	return data, nil
}

func (s *service) CompanyActivity() (interface{}, error) {
	data, err := s.store.GetCompanyActivity()
	if err != nil {
		return nil, response.ErrInternal.WithMessage("failed to fetch company activity")
	}
	return data, nil
}

func (s *service) Funnel() (interface{}, error) {
	data, err := s.store.GetPipelineFunnel()
	if err != nil {
		return nil, response.ErrInternal.WithMessage("failed to fetch funnel")
	}
	return data, nil
}

func (s *service) Location() (interface{}, error) {
	countries, err := s.store.GetLocationBreakdown()
	if err != nil {
		return nil, response.ErrInternal.WithMessage("failed to fetch location breakdown")
	}
	states, err := s.store.GetUSAStateBreakdown()
	if err != nil {
		return nil, response.ErrInternal.WithMessage("failed to fetch state breakdown")
	}
	return map[string]interface{}{
		"countries": countries,
		"states":    states,
	}, nil
}

func (s *service) Sponsorship(filter InsightsFilter) (interface{}, error) {
	days := filter.Days
	if days <= 0 {
		days = 30
	}
	data, err := s.store.GetSponsorshipTrend(days)
	if err != nil {
		return nil, response.ErrInternal.WithMessage("failed to fetch sponsorship trend")
	}
	return data, nil
}

func (s *service) Velocity() (interface{}, error) {
	data, err := s.store.GetCompanyVelocity(20)
	if err != nil {
		return nil, response.ErrInternal.WithMessage("failed to fetch company velocity")
	}
	return data, nil
}

func (s *service) RoleCategories() (interface{}, error) {
	data, err := s.store.GetRoleCategoryBreakdown()
	if err != nil {
		return nil, response.ErrInternal.WithMessage("failed to fetch role categories")
	}
	return data, nil
}

func (s *service) GetPipelineCounts() (PipelineCounts, error) {
	counts, err := s.store.GetPipelineCounts()
	if err != nil {
		return PipelineCounts{}, response.ErrInternal.WithMessage("failed to fetch pipeline counts")
	}
	return counts, nil
}
