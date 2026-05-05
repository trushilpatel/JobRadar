package application

import (
	"github.com/trushilpatel/jobradar/service/application/dto"
	"github.com/trushilpatel/jobradar/service/application/services"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	"gorm.io/gorm"
)

// Store is a compatibility alias for ApplicationRepository. Phase 2: the
// store interface (per-user, takes userID explicitly) is distinct from the
// user-facing ApplicationService — kept here only for legacy import paths
// until B7 migrates them.
type Store = services.ApplicationRepository

// Service is a compatibility alias for ApplicationService.
type Service = services.ApplicationService

// Handler is a compatibility alias for RestRouter.
type Handler = services.RestRouter

// ApplicationWithJob is a compatibility alias for ApplicationWithJob.
type ApplicationWithJob = services.ApplicationWithJob

// CreateApplicationInput is a compatibility alias for dto.CreateApplicationInput.
type CreateApplicationInput = dto.CreateApplicationInput

// UpdateStageInput is a compatibility alias for dto.UpdateStageInput.
type UpdateStageInput = dto.UpdateStageInput

// UpdateNoteInput is a compatibility alias for dto.UpdateNoteInput.
type UpdateNoteInput = dto.UpdateNoteInput

// NewStore constructs the per-user ApplicationRepository (terminal repo).
func NewStore(db *gorm.DB) services.ApplicationRepository {
	return services.NewApplicationRepositoryService(db)
}

// NewApplicationWriter constructs an ApplicationWriter backed by the repository.
func NewApplicationWriter(db *gorm.DB) jobsvc.ApplicationWriter {
	return services.NewApplicationWriter(db)
}
