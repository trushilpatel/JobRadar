package application

import (
	"github.com/trushilpatel/jobradar/service/application/dto"
	"github.com/trushilpatel/jobradar/service/application/services"
	"gorm.io/gorm"
)

// Store is a compatibility alias for ApplicationRepositoryService.
type Store = services.ApplicationRepositoryService

// Service is a compatibility alias for ApplicationService.
type Service = services.ApplicationService

// Handler is a compatibility alias for RestHandler.
type Handler = services.RestHandler

// Validator is a compatibility alias for ApplicationValidationService.
type Validator = services.ApplicationValidationService

// ApplicationWithJob is a compatibility alias for ApplicationWithJob.
type ApplicationWithJob = services.ApplicationWithJob

// CreateApplicationInput is a compatibility alias for dto.CreateApplicationInput.
type CreateApplicationInput = dto.CreateApplicationInput

// UpdateStageInput is a compatibility alias for dto.UpdateStageInput.
type UpdateStageInput = dto.UpdateStageInput

// UpdateNoteInput is a compatibility alias for dto.UpdateNoteInput.
type UpdateNoteInput = dto.UpdateNoteInput

// NewHandler wraps NewApplicationRestHandler for backward compatibility.
func NewHandler(v Validator, svc Service) *Handler {
	return services.NewApplicationRestHandler(v, svc)
}

// NewValidator wraps NewApplicationValidationService for backward compatibility.
func NewValidator() Validator {
	return services.NewApplicationValidationService()
}

// NewStore wraps NewApplicationRepositoryService for backward compatibility.
func NewStore(db *gorm.DB) Store {
	return services.NewApplicationRepositoryService(db)
}
