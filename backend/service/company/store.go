package company

import (
	"github.com/trushilpatel/jobradar/model"
	"gorm.io/gorm"
)

// CompanyReader is the interface the scan engine depends on.
// Workers receive this interface — not the concrete type.
type CompanyReader interface {
	GetActiveCompanies() ([]model.Company, error)
	GetAllCompanies() ([]model.Company, error)
	GetCompanyByID(id string) (model.Company, error)
	// GetCompaniesDueForScan returns active companies whose last_scan_at is NULL
	// or older than intervalMinutes ago. Used by the scheduler binary.
	GetCompaniesDueForScan(intervalMinutes int) ([]model.Company, error)
	// GetCompanies returns a filtered, paginated slice and the total matching count.
	GetCompanies(filter CompanyFilter, page, limit int) ([]model.Company, int64, error)
	// GetCompanyCounts returns (total, total_active) across all companies,
	// regardless of the current filter — used for header stats.
	GetCompanyCounts() (total, active int64, error error)
}

// CompanyWriter covers mutations from the API.
type CompanyWriter interface {
	CreateCompany(c model.Company) error
	UpdateCompany(id string, fields map[string]any) error
	DeleteCompany(id string) error
	SetAllCompaniesActive(active bool) error
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

func (s *companyStore) GetActiveCompanies() ([]model.Company, error) {
	var companies []model.Company
	err := s.db.Where("active = ?", true).Find(&companies).Error
	return companies, err
}

func (s *companyStore) GetAllCompanies() ([]model.Company, error) {
	var companies []model.Company
	err := s.db.Find(&companies).Error
	return companies, err
}

func (s *companyStore) GetCompanyByID(id string) (model.Company, error) {
	var company model.Company
	err := s.db.First(&company, "id = ?", id).Error
	if err != nil {
		return company, err
	}
	return company, nil
}

func (s *companyStore) CreateCompany(c model.Company) error {
	return s.db.Create(&c).Error
}

func (s *companyStore) UpdateCompany(id string, fields map[string]any) error {
	result := s.db.Model(&model.Company{}).Where("id = ?", id).Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *companyStore) DeleteCompany(id string) error {
	return s.db.Delete(&model.Company{}, "id = ?", id).Error
}

func (s *companyStore) SetAllCompaniesActive(active bool) error {
	return s.db.Model(&model.Company{}).Where("1 = 1").Update("active", active).Error
}

func (s *companyStore) GetCompaniesDueForScan(intervalMinutes int) ([]model.Company, error) {
	var companies []model.Company
	err := s.db.Where(
		"active = true AND (last_scan_at IS NULL OR last_scan_at + (? * INTERVAL '1 minute') <= NOW())",
		intervalMinutes,
	).Find(&companies).Error
	return companies, err
}

func (s *companyStore) GetCompanies(filter CompanyFilter, page, limit int) ([]model.Company, int64, error) {
	q := s.db.Model(&model.Company{})

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
	err := q.Order("name ASC").Offset(offset).Limit(limit).Find(&companies).Error
	return companies, total, err
}

func (s *companyStore) GetCompanyCounts() (total, active int64, err error) {
	if err = s.db.Model(&model.Company{}).Count(&total).Error; err != nil {
		return
	}
	err = s.db.Model(&model.Company{}).Where("active = ?", true).Count(&active).Error
	return
}
