package services

import "github.com/trushilpatel/jobradar/common/domain"

// DashboardValidationService validates user-submitted input for the dashboard domain.
type DashboardValidationService interface {
	ValidateActivity(days int) error
}

type dashboardValidationService struct{}

// NewDashboardValidationService constructs a DashboardValidationService.
func NewDashboardValidationService() DashboardValidationService {
	return &dashboardValidationService{}
}

// ValidateActivity validates that days >= 1.
func (v *dashboardValidationService) ValidateActivity(days int) error {
	if days < 1 {
		return &domain.ValidationError{
			Field:   "days",
			Code:    "INVALID_VALUE",
			Message: "days must be >= 1",
		}
	}
	return nil
}
