package insights

import (
	"context"
	"errors"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/insights/dto"
	svc "github.com/trushilpatel/jobradar/service/insights/services"
)

type noopInsightsService struct{}

func (n *noopInsightsService) TechStack(_ context.Context) ([]dto.TechCount, error) {
	return nil, nil
}
func (n *noopInsightsService) CompanyActivity(_ context.Context) ([]dto.CompanyActivityItem, error) {
	return nil, nil
}
func (n *noopInsightsService) Funnel(_ context.Context) ([]dto.FunnelStep, error) { return nil, nil }
func (n *noopInsightsService) Location(_ context.Context) (dto.LocationBreakdown, error) {
	return dto.LocationBreakdown{}, nil
}
func (n *noopInsightsService) Sponsorship(_ context.Context, _ dto.InsightsFilter) ([]dto.SponsorshipDay, error) {
	return []dto.SponsorshipDay{}, nil
}
func (n *noopInsightsService) Velocity(_ context.Context) ([]dto.CompanyVelocity, error) {
	return nil, nil
}
func (n *noopInsightsService) RoleCategories(_ context.Context) ([]dto.RoleCategoryCount, error) {
	return nil, nil
}
func (n *noopInsightsService) GetPipelineCounts(_ context.Context) (dto.PipelineCounts, error) {
	return dto.PipelineCounts{}, nil
}

func TestValidateFilter(t *testing.T) {
	v := svc.NewValidationService(&noopInsightsService{})

	_, err := v.Sponsorship(context.Background(), dto.InsightsFilter{Days: 1})
	if err != nil {
		t.Errorf("unexpected error for days=1: %v", err)
	}

	_, err = v.Sponsorship(context.Background(), dto.InsightsFilter{Days: 30})
	if err != nil {
		t.Errorf("unexpected error for days=30: %v", err)
	}

	_, err = v.Sponsorship(context.Background(), dto.InsightsFilter{Days: 0})
	if err == nil {
		t.Error("expected error for days=0")
	}
	var vErrs apperr.ValidationErrors
	if !errors.As(err, &vErrs) {
		t.Errorf("expected ValidationErrors, got %T: %v", err, err)
	}
	if len(vErrs) != 1 {
		t.Errorf("expected 1 validation error, got %d", len(vErrs))
	}
	if len(vErrs) > 0 && vErrs[0].Field != "days" {
		t.Errorf("expected field 'days', got %q", vErrs[0].Field)
	}

	_, err = v.Sponsorship(context.Background(), dto.InsightsFilter{Days: -1})
	if err == nil {
		t.Error("expected error for days=-1")
	}
}

var _ svc.InsightsService = (*noopInsightsService)(nil)
