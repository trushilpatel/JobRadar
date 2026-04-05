package store

import (
	"github.com/trushilpatel/jobradar/internal/model"
	"gorm.io/gorm"
)

// CompanyReader is the interface the scan engine depends on.
// Workers receive this interface — not the concrete type.
type CompanyReader interface {
	GetActiveCompanies() ([]model.Company, error)
}

// companyStore is the private implementation. Only NewCompanyStore can create it.
type companyStore struct {
	db *gorm.DB
}

func NewCompanyStore(db *gorm.DB) CompanyReader {
	return &companyStore{db: db}
}

func (s *companyStore) GetActiveCompanies() ([]model.Company, error) {
	var companies []model.Company
	err := s.db.Where("active = ?", true).Find(&companies).Error
	return companies, err
}
