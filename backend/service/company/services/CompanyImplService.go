package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/common/domain"
	"github.com/trushilpatel/jobradar/infra/tracing"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	"github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	"github.com/trushilpatel/jobradar/store"
)

type service struct {
	store       Store
	config      common.Config
	scanHistory companydto.ScanHistoryReader
}

// NewService constructs a Service implementation.
func NewService(s Store, cfg common.Config, scanHistory companydto.ScanHistoryReader) Service {
	return &service{store: s, config: cfg, scanHistory: scanHistory}
}

// List returns a filtered, paginated page of companies enriched with next_scan_at
// and scan_history (last ≤10 scan outcomes from company_scan_log).
func (s *service) List(ctx context.Context, filter companydto.CompanyFilter, page, limit int) (_ companydto.CompanyListResult, err error) {
	ctx, span := tracing.Start(ctx, "company.service.List")
	defer tracing.End(span, &err)
	companies, total, err := s.store.GetCompanies(ctx, filter, page, limit)
	if err != nil {
		return companydto.CompanyListResult{}, domain.ErrInternal.WithMessage("failed to fetch companies")
	}

	_, totalActive, err := s.store.GetCompanyCounts(ctx)
	if err != nil {
		return companydto.CompanyListResult{}, domain.ErrInternal.WithMessage("failed to fetch company counts")
	}

	// Fetch real scan history for this page of companies in a single query.
	ids := make([]string, len(companies))
	for i, c := range companies {
		ids[i] = c.ID
	}
	histories, err := s.scanHistory.GetScanHistories(ctx, ids)
	if err != nil {
		// Non-fatal: degrade gracefully — scan history is cosmetic.
		histories = map[string][]bool{}
	}

	views := make([]companydto.CompanyView, 0, len(companies))
	for _, c := range companies {
		v := companydto.CompanyView{Company: c}
		if c.LastScanAt != nil {
			t := c.LastScanAt.Add(time.Duration(s.config.ScanIntervalMinutes) * time.Minute)
			v.NextScanAt = &t
		}
		v.ScanHistory = histories[c.ID] // nil if no runs yet
		views = append(views, v)
	}

	return companydto.CompanyListResult{
		Items:       views,
		Total:       total,
		TotalActive: totalActive,
	}, nil
}

// GetAll returns all companies as views without pagination (for internal/worker use).
func (s *service) GetAll(ctx context.Context) (_ []companydto.CompanyView, err error) {
	ctx, span := tracing.Start(ctx, "company.service.GetAll")
	defer tracing.End(span, &err)
	companies, err := s.store.GetAllCompanies(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch companies")
	}
	views := make([]companydto.CompanyView, 0, len(companies))
	for _, c := range companies {
		v := companydto.CompanyView{Company: c}
		if c.LastScanAt != nil {
			t := c.LastScanAt.Add(time.Duration(s.config.ScanIntervalMinutes) * time.Minute)
			v.NextScanAt = &t
		}
		views = append(views, v)
	}
	return views, nil
}

// Get fetches a single company by ID.
func (s *service) Get(ctx context.Context, id string) (_ *companymodel.Company, err error) {
	ctx, span := tracing.Start(ctx, "company.service.Get")
	defer tracing.End(span, &err)
	c, err := s.store.GetCompanyByID(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, domain.ErrNotFound.WithMessage("company not found")
		}
		return nil, domain.ErrInternal.WithMessage("failed to fetch company")
	}
	return &c, nil
}

// Create inserts a new company. Sets last_scan_at to now so the scheduler does not
// immediately pick it up — users trigger the first scan via POST /companies/:id/scan.
func (s *service) Create(ctx context.Context, input companydto.CreateCompanyInput) (_ *companymodel.Company, err error) {
	ctx, span := tracing.Start(ctx, "company.service.Create")
	defer tracing.End(span, &err)
	active := true
	if input.Active != nil {
		active = *input.Active
	}

	now := time.Now()
	company := companymodel.Company{
		ID:          input.ID,
		Name:        input.Name,
		URL:         input.URL,
		AdapterType: input.AdapterType,
		Active:      active,
		LastScanAt:  &now,
	}

	if err := s.store.CreateCompany(ctx, company); err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to create company")
	}
	return &company, nil
}

// Update applies a partial patch to an existing company. Nil fields are skipped.
func (s *service) Update(ctx context.Context, id string, input companydto.UpdateCompanyInput) (err error) {
	ctx, span := tracing.Start(ctx, "company.service.Update")
	defer tracing.End(span, &err)
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

	if err := s.store.UpdateCompany(ctx, id, fields); err != nil {
		if store.IsNotFound(err) {
			return domain.ErrNotFound.WithMessage("company not found")
		}
		return domain.ErrInternal.WithMessage("failed to update company")
	}
	return nil
}

// Delete removes a company after verifying it exists.
func (s *service) Delete(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "company.service.Delete")
	defer tracing.End(span, &err)
	if _, err := s.store.GetCompanyByID(ctx, id); err != nil {
		if store.IsNotFound(err) {
			return domain.ErrNotFound.WithMessage("company not found")
		}
		return domain.ErrInternal.WithMessage("failed to fetch company")
	}
	if err := s.store.DeleteCompany(ctx, id); err != nil {
		return domain.ErrInternal.WithMessage("failed to delete company")
	}
	return nil
}

// SetAllActive sets every company's active flag to the given value.
func (s *service) SetAllActive(ctx context.Context, active bool) (err error) {
	ctx, span := tracing.Start(ctx, "company.service.SetAllActive")
	defer tracing.End(span, &err)
	if err := s.store.SetAllCompaniesActive(ctx, active); err != nil {
		return domain.ErrInternal.WithMessage("failed to update companies")
	}
	return nil
}
