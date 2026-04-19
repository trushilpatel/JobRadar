package company

import (
	"github.com/trushilpatel/jobradar/service/company/services"
	"gorm.io/gorm"
)

// Backward compatibility: re-export services constructors and interfaces

type Handler = services.Handler
type Validator = services.Validator
type Service = services.Service
type Store = services.Store
type CompanyReader = services.CompanyReader
type CompanyWriter = services.CompanyWriter

func NewHandler(v Validator, svc Service) *Handler {
	return services.NewHandler(v, svc)
}

func NewValidator() Validator {
	return services.NewValidator()
}

func NewService(s Store, cfg Config, scanHistory ScanHistoryReader) Service {
	return services.NewService(s, cfg, scanHistory)
}

func NewStore(db *gorm.DB) Store {
	return services.NewStore(db)
}
