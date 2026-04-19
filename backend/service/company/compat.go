package company

import (
	"github.com/trushilpatel/jobradar/service/company/services"
	"gorm.io/gorm"
)

// Backward compatibility: re-export services constructors and interfaces.

type Handler = services.RestRouter
type Service = services.Service
type Store = services.Store
type CompanyReader = services.CompanyReader
type CompanyWriter = services.CompanyWriter

func NewHandler(h *services.RestService) *Handler {
	return services.NewRestRouter(h)
}

func NewValidator(next Service) Service {
	return services.NewValidator(next)
}

func NewService(next Service, cfg Config, scanHistory ScanHistoryReader) Service {
	return services.NewService(next, cfg, scanHistory)
}

func NewStore(db *gorm.DB) Store {
	return services.NewStore(db)
}
