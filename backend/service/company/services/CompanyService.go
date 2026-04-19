package services

import (
	"context"

	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
)

// Validator validates user-submitted input for the company domain.
// It never makes DB calls.
type Validator interface {
	ValidateList(filter companydto.CompanyFilter, page, limit int) error
	ValidateCreate(input companydto.CreateCompanyInput) error
	ValidateUpdate(input companydto.UpdateCompanyInput) error
	ValidateSetAllActive(active *bool) error
}

// Service defines all business operations for the company domain.
type Service interface {
	List(ctx context.Context, filter companydto.CompanyFilter, page, limit int) (companydto.CompanyListResult, error)
	// GetAll returns every company as a view without pagination.
	// Used by internal workers that need the full set (e.g. scan queue enrichment).
	GetAll(ctx context.Context) ([]companydto.CompanyView, error)
	Get(ctx context.Context, id string) (*companymodel.Company, error)
	Create(ctx context.Context, input companydto.CreateCompanyInput) (*companymodel.Company, error)
	Update(ctx context.Context, id string, input companydto.UpdateCompanyInput) error
	Delete(ctx context.Context, id string) error
	SetAllActive(ctx context.Context, active bool) error
}
