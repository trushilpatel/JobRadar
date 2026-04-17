package company

import (
	"context"

	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/model"
	"gorm.io/gorm"
)

// CompanyReader is the interface the scan engine depends on.
// Workers receive this interface — not the concrete type.
type CompanyReader interface {
	GetActiveCompanies(ctx context.Context) ([]model.Company, error)
	GetAllCompanies(ctx context.Context) ([]model.Company, error)
	GetCompanyByID(ctx context.Context, id string) (model.Company, error)
	// GetCompaniesDueForScan returns active companies whose last_scan_at is NULL
	// or older than intervalMinutes ago. Used by the scheduler binary.
	GetCompaniesDueForScan(ctx context.Context, intervalMinutes int) ([]model.Company, error)
	// GetCompanies returns a filtered, paginated slice and the total matching count.
	GetCompanies(ctx context.Context, filter CompanyFilter, page, limit int) ([]model.Company, int64, error)
	// GetCompanyCounts returns (total, total_active) across all companies,
	// regardless of the current filter — used for header stats.
	GetCompanyCounts(ctx context.Context) (total, active int64, error error)
}

// CompanyWriter covers mutations from the API.
type CompanyWriter interface {
	CreateCompany(ctx context.Context, c model.Company) error
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

func (s *companyStore) GetActiveCompanies(ctx context.Context) (companies []model.Company, err error) {
	ctx, span := tracing.Start(ctx, "company.store.GetActiveCompanies")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Where("active = ?", true).Find(&companies).Error
	return companies, err
}

func (s *companyStore) GetAllCompanies(ctx context.Context) (companies []model.Company, err error) {
	ctx, span := tracing.Start(ctx, "company.store.GetAllCompanies")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Find(&companies).Error
	return companies, err
}

func (s *companyStore) GetCompanyByID(ctx context.Context, id string) (_ model.Company, err error) {
	ctx, span := tracing.Start(ctx, "company.store.GetCompanyByID")
	defer tracing.End(span, &err)
	var company model.Company
	err = s.db.WithContext(ctx).First(&company, "id = ?", id).Error
	if err != nil {
		return company, err
	}
	return company, nil
}

func (s *companyStore) CreateCompany(ctx context.Context, c model.Company) (err error) {
	ctx, span := tracing.Start(ctx, "company.store.CreateCompany")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Create(&c).Error
}

func (s *companyStore) UpdateCompany(ctx context.Context, id string, fields map[string]any) (err error) {
	ctx, span := tracing.Start(ctx, "company.store.UpdateCompany")
	defer tracing.End(span, &err)
	result := s.db.WithContext(ctx).Model(&model.Company{}).Where("id = ?", id).Updates(fields)
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
	return s.db.WithContext(ctx).Delete(&model.Company{}, "id = ?", id).Error
}

func (s *companyStore) SetAllCompaniesActive(ctx context.Context, active bool) (err error) {
	ctx, span := tracing.Start(ctx, "company.store.SetAllCompaniesActive")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Model(&model.Company{}).Where("1 = 1").Update("active", active).Error
}

func (s *companyStore) GetCompaniesDueForScan(ctx context.Context, intervalMinutes int) (companies []model.Company, err error) {
	ctx, span := tracing.Start(ctx, "company.store.GetCompaniesDueForScan")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Where(
		"active = true AND (last_scan_at IS NULL OR last_scan_at + (? * INTERVAL '1 minute') <= NOW())",
		intervalMinutes,
	).Find(&companies).Error
	return companies, err
}

func (s *companyStore) GetCompanies(ctx context.Context, filter CompanyFilter, page, limit int) (_ []model.Company, _ int64, err error) {
	ctx, span := tracing.Start(ctx, "company.store.GetCompanies")
	defer tracing.End(span, &err)
	q := s.db.WithContext(ctx).Model(&model.Company{})

	if filter.Active != nil {
		q = q.Where("active = ?", *filter.Active)
	}
	if filter.AdapterType != "" {
		q = q.Where("adapter_type = ?", filter.AdapterType)
	}
	if filter.Search != "" {
		q = q.Where("name ILIKE ?", "%"+filter.Search+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var companies []model.Company
	offset := (page - 1) * limit
	err = q.Order("name ASC").Offset(offset).Limit(limit).Find(&companies).Error
	return companies, total, err
}

func (s *companyStore) GetCompanyCounts(ctx context.Context) (total, active int64, err error) {
	ctx, span := tracing.Start(ctx, "company.store.GetCompanyCounts")
	defer tracing.End(span, &err)
	if err = s.db.WithContext(ctx).Model(&model.Company{}).Count(&total).Error; err != nil {
		return
	}
	err = s.db.WithContext(ctx).Model(&model.Company{}).Where("active = ?", true).Count(&active).Error
	return
}

func (s *companyStore) UpdateScanStatus(ctx context.Context, id, status string) (err error) {
	ctx, span := tracing.Start(ctx, "company.store.UpdateScanStatus")
	defer tracing.End(span, &err)
	result := s.db.WithContext(ctx).Model(&model.Company{}).
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
