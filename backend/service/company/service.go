package company

import (
	"time"

	"github.com/trushilpatel/jobradar/api/response"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/store"
)

// Service defines all business operations for the company domain.
type Service interface {
	List(filter CompanyFilter) ([]CompanyView, error)
	Get(id string) (*model.Company, error)
	Create(input CreateCompanyInput) (*model.Company, error)
	Update(id string, input UpdateCompanyInput) error
	Delete(id string) error
	SetAllActive(active bool) error
}

type service struct {
	store  Store
	config Config
}

// NewService constructs a Service implementation.
func NewService(s Store, cfg Config) Service {
	return &service{store: s, config: cfg}
}

// List returns all companies enriched with computed next_scan_at.
func (s *service) List(filter CompanyFilter) ([]CompanyView, error) {
	companies, err := s.store.GetAllCompanies()
	if err != nil {
		return nil, response.ErrInternal.WithMessage("failed to fetch companies")
	}

	views := make([]CompanyView, 0, len(companies))
	for _, c := range companies {
		// Apply active filter if set
		if filter.Active != nil && c.Active != *filter.Active {
			continue
		}
		// Apply adapter_type filter if set
		if filter.AdapterType != "" && c.AdapterType != filter.AdapterType {
			continue
		}

		v := CompanyView{Company: c}
		if c.LastScanAt != nil {
			t := c.LastScanAt.Add(time.Duration(s.config.ScanIntervalMinutes) * time.Minute)
			v.NextScanAt = &t
		}
		views = append(views, v)
	}

	return views, nil
}

// Get fetches a single company by ID.
func (s *service) Get(id string) (*model.Company, error) {
	c, err := s.store.GetCompanyByID(id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, response.ErrNotFound.WithMessage("company not found")
		}
		return nil, response.ErrInternal.WithMessage("failed to fetch company")
	}
	return &c, nil
}

// Create inserts a new company. Sets last_scan_at to now so the scheduler does not
// immediately pick it up — users trigger the first scan via POST /companies/:id/scan.
func (s *service) Create(input CreateCompanyInput) (*model.Company, error) {
	active := true
	if input.Active != nil {
		active = *input.Active
	}

	now := time.Now()
	company := model.Company{
		ID:          input.ID,
		Name:        input.Name,
		URL:         input.URL,
		AdapterType: input.AdapterType,
		Active:      active,
		LastScanAt:  &now,
	}

	if err := s.store.CreateCompany(company); err != nil {
		return nil, response.ErrInternal.WithMessage("failed to create company")
	}
	return &company, nil
}

// Update applies a partial patch to an existing company. Nil fields are skipped.
func (s *service) Update(id string, input UpdateCompanyInput) error {
	fields := make(map[string]any)
	if input.Name != nil {
		fields["name"] = *input.Name
	}
	if input.URL != nil {
		fields["url"] = *input.URL
	}
	if input.AdapterType != nil {
		fields["adapter_type"] = *input.AdapterType
	}
	if input.Active != nil {
		fields["active"] = *input.Active
	}

	if err := s.store.UpdateCompany(id, fields); err != nil {
		if store.IsNotFound(err) {
			return response.ErrNotFound.WithMessage("company not found")
		}
		return response.ErrInternal.WithMessage("failed to update company")
	}
	return nil
}

// Delete removes a company after verifying it exists.
func (s *service) Delete(id string) error {
	if _, err := s.store.GetCompanyByID(id); err != nil {
		if store.IsNotFound(err) {
			return response.ErrNotFound.WithMessage("company not found")
		}
		return response.ErrInternal.WithMessage("failed to fetch company")
	}
	if err := s.store.DeleteCompany(id); err != nil {
		return response.ErrInternal.WithMessage("failed to delete company")
	}
	return nil
}

// SetAllActive sets every company's active flag to the given value.
func (s *service) SetAllActive(active bool) error {
	if err := s.store.SetAllCompaniesActive(active); err != nil {
		return response.ErrInternal.WithMessage("failed to update companies")
	}
	return nil
}
