package insights

import (
	"context"
	"errors"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/insights/dto"
	svc "github.com/trushilpatel/jobradar/service/insights/services"
)

// noopInsightsService is a no-op test double for InsightsService used as the "next" in validation tests.
type noopInsightsService struct{}

func (n *noopInsightsService) TechStack(_ context.Context) (interface{}, error) { return nil, nil }
func (n *noopInsightsService) CompanyActivity(_ context.Context) (interface{}, error) {
	return nil, nil
}
func (n *noopInsightsService) Funnel(_ context.Context) (interface{}, error)   { return nil, nil }
func (n *noopInsightsService) Location(_ context.Context) (interface{}, error) { return nil, nil }
func (n *noopInsightsService) Sponsorship(_ context.Context, _ dto.InsightsFilter) (interface{}, error) {
	return []dto.SponsorshipDay{}, nil
}
func (n *noopInsightsService) Velocity(_ context.Context) (interface{}, error)       { return nil, nil }
func (n *noopInsightsService) RoleCategories(_ context.Context) (interface{}, error) { return nil, nil }
func (n *noopInsightsService) GetPipelineCounts(_ context.Context) (dto.PipelineCounts, error) {
	return dto.PipelineCounts{}, nil
}

func TestValidateFilter(t *testing.T) {
	v := svc.NewValidationService(&noopInsightsService{})

	// valid days — should pass through without error
	_, err := v.Sponsorship(context.Background(), dto.InsightsFilter{Days: 1})
	if err != nil {
		t.Errorf("unexpected error for days=1: %v", err)
	}

	_, err = v.Sponsorship(context.Background(), dto.InsightsFilter{Days: 30})
	if err != nil {
		t.Errorf("unexpected error for days=30: %v", err)
	}

	// invalid days — should return ValidationError
	_, err = v.Sponsorship(context.Background(), dto.InsightsFilter{Days: 0})
	if err == nil {
		t.Error("expected error for days=0")
	}
	var vErr *apperr.ValidationError
	if !errors.As(err, &vErr) {
		t.Errorf("expected ValidationError, got %T: %v", err, err)
	}

	_, err = v.Sponsorship(context.Background(), dto.InsightsFilter{Days: -1})
	if err == nil {
		t.Error("expected error for days=-1")
	}
}

// Ensure test double implements InsightsService.
var _ svc.InsightsService = (*noopInsightsService)(nil)
