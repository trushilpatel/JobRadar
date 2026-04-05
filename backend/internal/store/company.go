package store

import (
	"github.com/trushilpatel/jobradar/internal/model"
	"gorm.io/gorm"
)

// CompanyReader is the interface the scan engine depends on.
// Workers receive this interface — not the concrete type.
type CompanyReader interface {
	GetActiveCompanies() ([]model.Company, error)
	GetAllCompanies() ([]model.Company, error)
	GetCompanyByID(id string) (model.Company, error)
}

// CompanyWriter covers mutations from the API.
type CompanyWriter interface {
	CreateCompany(c model.Company) error
	UpdateCompany(id string, fields map[string]any) error
	DeleteCompany(id string) error
}

// CompanyStore is the full interface used by the API handler.
type CompanyStore interface {
	CompanyReader
	CompanyWriter
}

// companyStore is the private implementation. Only NewCompanyStore can create it.
type companyStore struct {
	db *gorm.DB
}

func NewCompanyStore(db *gorm.DB) CompanyStore {
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
	return s.db.Model(&model.Company{}).Where("id = ?", id).Updates(fields).Error
}

func (s *companyStore) DeleteCompany(id string) error {
	return s.db.Delete(&model.Company{}, "id = ?", id).Error
}
