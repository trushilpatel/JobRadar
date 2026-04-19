package services

import (
	"context"

	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
)

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

// BaseService auto-delegates all Service methods to next.
// Embed this in decorators and override only the methods they own.
type BaseService struct {
	next Service
}

// NewBaseService constructs a BaseService wrapping next.
// Panics if next is nil to surface wiring mistakes early.
func NewBaseService(next Service) BaseService {
	if next == nil {
		panic("BaseService: next must not be nil — check wire setup")
	}
	return BaseService{next: next}
}

func (b *BaseService) List(ctx context.Context, filter companydto.CompanyFilter, page, limit int) (companydto.CompanyListResult, error) {
	return b.next.List(ctx, filter, page, limit)
}

func (b *BaseService) GetAll(ctx context.Context) ([]companydto.CompanyView, error) {
	return b.next.GetAll(ctx)
}

func (b *BaseService) Get(ctx context.Context, id string) (*companymodel.Company, error) {
	return b.next.Get(ctx, id)
}

func (b *BaseService) Create(ctx context.Context, input companydto.CreateCompanyInput) (*companymodel.Company, error) {
	return b.next.Create(ctx, input)
}

func (b *BaseService) Update(ctx context.Context, id string, input companydto.UpdateCompanyInput) error {
	return b.next.Update(ctx, id, input)
}

func (b *BaseService) Delete(ctx context.Context, id string) error {
	return b.next.Delete(ctx, id)
}

func (b *BaseService) SetAllActive(ctx context.Context, active bool) error {
	return b.next.SetAllActive(ctx, active)
}
