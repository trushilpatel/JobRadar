package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

// CompanyReader is the interface the scan engine depends on.
// Workers receive this interface — not the concrete type.
type CompanyReader interface {
	GetActiveCompanies(ctx context.Context) ([]companymodel.Company, error)
	GetAllCompanies(ctx context.Context) ([]companymodel.Company, error)
	GetCompanyByID(ctx context.Context, id string) (companymodel.Company, error)
	// GetCompanies returns a filtered, paginated slice and the total matching count.
	GetCompanies(ctx context.Context, filter companydto.CompanyFilter, page, limit int) ([]companymodel.Company, int64, error)
	// GetCompanyCounts returns (total, total_active) across all companies,
	// regardless of the current filter — used for header stats.
	GetCompanyCounts(ctx context.Context) (total, active int64, error error)
	// GetCompaniesByIDs returns only the companies whose IDs are in the given slice.
	// Returns an empty slice (not an error) when ids is empty.
	GetCompaniesByIDs(ctx context.Context, ids []string) ([]companymodel.Company, error)
}

// CompanyWriter covers mutations from the API.
type CompanyWriter interface {
	CreateCompany(ctx context.Context, c companymodel.Company) error
	UpdateCompany(ctx context.Context, id string, fields map[string]any) error
	DeleteCompany(ctx context.Context, id string) error
	SetAllCompaniesActive(ctx context.Context, active bool) error
	// UpdateScanStatus records the result of a completed scan on the company row.
	// It updates last_scan_at to now and sets last_scan_status to the given status.
	UpdateScanStatus(ctx context.Context, id, status string) error
}

// Store is the full interface for the company domain.
type Store interface {
	CompanyReader
	CompanyWriter
}

type companyStore struct {
	db *gorm.DB
}

// NewStore constructs a Store implementation backed by GORM.
func NewStore(db *gorm.DB) Store {
	return &companyStore{db: db}
}

func (s *companyStore) GetActiveCompanies(ctx context.Context) (companies []companymodel.Company, err error) {
	ctx, span := tracing.Start(ctx, "company.store.GetActiveCompanies")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Where("active = ?", true).Find(&companies).Error
	return companies, err
}

func (s *companyStore) GetAllCompanies(ctx context.Context) (companies []companymodel.Company, err error) {
	ctx, span := tracing.Start(ctx, "company.store.GetAllCompanies")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Find(&companies).Error
	return companies, err
}

func (s *companyStore) GetCompanyByID(ctx context.Context, id string) (_ companymodel.Company, err error) {
	ctx, span := tracing.Start(ctx, "company.store.GetCompanyByID")
	defer tracing.End(span, &err)
	var company companymodel.Company
	err = s.db.WithContext(ctx).First(&company, "id = ?", id).Error
	if err != nil {
		return company, err
	}
	return company, nil
}

func (s *companyStore) CreateCompany(ctx context.Context, c companymodel.Company) (err error) {
	ctx, span := tracing.Start(ctx, "company.store.CreateCompany")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Create(&c).Error
}

func (s *companyStore) UpdateCompany(ctx context.Context, id string, fields map[string]any) (err error) {
	ctx, span := tracing.Start(ctx, "company.store.UpdateCompany")
	defer tracing.End(span, &err)
	result := s.db.WithContext(ctx).Model(&companymodel.Company{}).Where("id = ?", id).Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *companyStore) DeleteCompany(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "company.store.DeleteCompany")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Delete(&companymodel.Company{}, "id = ?", id).Error
}

func (s *companyStore) SetAllCompaniesActive(ctx context.Context, active bool) (err error) {
	ctx, span := tracing.Start(ctx, "company.store.SetAllCompaniesActive")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Model(&companymodel.Company{}).Where("1 = 1").Update("active", active).Error
}

func (s *companyStore) GetCompanies(ctx context.Context, filter companydto.CompanyFilter, page, limit int) (_ []companymodel.Company, _ int64, err error) {
	ctx, span := tracing.Start(ctx, "company.store.GetCompanies")
	defer tracing.End(span, &err)
	q := s.db.WithContext(ctx).Model(&companymodel.Company{})

	if filter.Active != nil {
		q = q.Where("active = ?", *filter.Active)
	}
	if len(filter.AdapterTypes) > 0 {
		q = q.Where("adapter_type IN ?", filter.AdapterTypes)
	}
	if filter.Search != "" {
		q = q.Where("name ILIKE ?", "%"+filter.Search+"%")
	}
	if filter.LastScanStatus != "" {
		q = q.Where("last_scan_status = ?", filter.LastScanStatus)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Whitelist sort columns to prevent SQL injection.
	var col string
	switch filter.SortBy {
	case "adapter_type":
		col = "adapter_type"
	case "last_scan_at":
		col = "last_scan_at"
	default:
		col = "name"
	}
	dir := "ASC"
	if filter.SortDir == "desc" {
		dir = "DESC"
	}

	var companies []companymodel.Company
	offset := (page - 1) * limit
	err = q.Order(col + " " + dir).Offset(offset).Limit(limit).Find(&companies).Error
	return companies, total, err
}

func (s *companyStore) GetCompanyCounts(ctx context.Context) (total, active int64, err error) {
	ctx, span := tracing.Start(ctx, "company.store.GetCompanyCounts")
	defer tracing.End(span, &err)
	var counts struct {
		Total  int64
		Active int64
	}
	err = s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*)                       AS total,
			COUNT(*) FILTER (WHERE active) AS active
		FROM companies
	`).Scan(&counts).Error
	return counts.Total, counts.Active, err
}

func (s *companyStore) GetCompaniesByIDs(ctx context.Context, ids []string) (_ []companymodel.Company, err error) {
	ctx, span := tracing.Start(ctx, "company.store.GetCompaniesByIDs")
	defer tracing.End(span, &err)
	if len(ids) == 0 {
		return nil, nil
	}
	var companies []companymodel.Company
	err = s.db.WithContext(ctx).Where("id IN ?", ids).Find(&companies).Error
	return companies, err
}

func (s *companyStore) UpdateScanStatus(ctx context.Context, id, status string) (err error) {
	ctx, span := tracing.Start(ctx, "company.store.UpdateScanStatus")
	defer tracing.End(span, &err)
	result := s.db.WithContext(ctx).Model(&companymodel.Company{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_scan_at":     gorm.Expr("NOW()"),
			"last_scan_status": status,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// repositoryService is the terminal layer of the decorator chain.
// It implements all Service methods by delegating directly to the Store.
// It has no next and does not embed BaseService.
type repositoryService struct {
	store Store
}

// NewRepositoryService constructs the terminal Service backed by a Store.
func NewRepositoryService(s Store) Service {
	return &repositoryService{store: s}
}

func (r *repositoryService) List(ctx context.Context, filter companydto.CompanyFilter, page, limit int) (_ companydto.CompanyListResult, err error) {
	ctx, span := tracing.Start(ctx, "company.repository.List")
	defer tracing.End(span, &err)

	var (
		companies   []companymodel.Company
		total       int64
		totalActive int64
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var e error
		companies, total, e = r.store.GetCompanies(gctx, filter, page, limit)
		return e
	})

	g.Go(func() error {
		var e error
		_, totalActive, e = r.store.GetCompanyCounts(gctx)
		return e
	})

	if err = g.Wait(); err != nil {
		return companydto.CompanyListResult{}, errors.ErrInternal.WithMessage("failed to fetch companies")
	}

	views := make([]companydto.CompanyView, 0, len(companies))
	for _, c := range companies {
		views = append(views, companydto.CompanyView{Company: c})
	}
	return companydto.CompanyListResult{
		Items:       views,
		Total:       total,
		TotalActive: totalActive,
	}, nil
}

func (r *repositoryService) GetAll(ctx context.Context) (_ []companydto.CompanyView, err error) {
	ctx, span := tracing.Start(ctx, "company.repository.GetAll")
	defer tracing.End(span, &err)
	companies, err := r.store.GetAllCompanies(ctx)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch companies")
	}
	views := make([]companydto.CompanyView, 0, len(companies))
	for _, c := range companies {
		views = append(views, companydto.CompanyView{Company: c})
	}
	return views, nil
}

func (r *repositoryService) Get(ctx context.Context, id string) (_ *companymodel.Company, err error) {
	ctx, span := tracing.Start(ctx, "company.repository.Get")
	defer tracing.End(span, &err)
	c, err := r.store.GetCompanyByID(ctx, id)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.ErrNotFound.WithMessage("company not found")
		}
		return nil, errors.ErrInternal.WithMessage("failed to fetch company")
	}
	return &c, nil
}

func (r *repositoryService) Create(ctx context.Context, input companydto.CreateCompanyInput) (_ *companymodel.Company, err error) {
	ctx, span := tracing.Start(ctx, "company.repository.Create")
	defer tracing.End(span, &err)
	active := false
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
	if err := r.store.CreateCompany(ctx, company); err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to create company")
	}
	return &company, nil
}

func (r *repositoryService) Update(ctx context.Context, id string, input companydto.UpdateCompanyInput) (err error) {
	ctx, span := tracing.Start(ctx, "company.repository.Update")
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
	if err := r.store.UpdateCompany(ctx, id, fields); err != nil {
		if errors.IsNotFound(err) {
			return errors.ErrNotFound.WithMessage("company not found")
		}
		return errors.ErrInternal.WithMessage("failed to update company")
	}
	return nil
}

func (r *repositoryService) Delete(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "company.repository.Delete")
	defer tracing.End(span, &err)
	if _, err := r.store.GetCompanyByID(ctx, id); err != nil {
		if errors.IsNotFound(err) {
			return errors.ErrNotFound.WithMessage("company not found")
		}
		return errors.ErrInternal.WithMessage("failed to fetch company")
	}
	if err := r.store.DeleteCompany(ctx, id); err != nil {
		return errors.ErrInternal.WithMessage("failed to delete company")
	}
	return nil
}

func (r *repositoryService) GetByIDs(ctx context.Context, ids []string) (_ []companydto.CompanyView, err error) {
	ctx, span := tracing.Start(ctx, "company.repository.GetByIDs")
	defer tracing.End(span, &err)
	companies, err := r.store.GetCompaniesByIDs(ctx, ids)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch companies by IDs")
	}
	views := make([]companydto.CompanyView, 0, len(companies))
	for _, c := range companies {
		views = append(views, companydto.CompanyView{Company: c})
	}
	return views, nil
}

func (r *repositoryService) SetAllActive(ctx context.Context, active bool) (err error) {
	ctx, span := tracing.Start(ctx, "company.repository.SetAllActive")
	defer tracing.End(span, &err)
	if err := r.store.SetAllCompaniesActive(ctx, active); err != nil {
		return errors.ErrInternal.WithMessage("failed to update companies")
	}
	return nil
}
