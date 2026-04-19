package services

import (
	"context"

	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	"github.com/trushilpatel/jobradar/service/application/dto"
)

// ApplicationWithJob joins a JobApplication with its Job title and Company name.
type ApplicationWithJob struct {
	appmodel.JobApplication
	JobTitle    string `json:"job_title"`
	CompanyID   string `json:"company_id"`
	CompanyName string `json:"company_name"`
}

// ApplicationService defines all business operations for the application domain.
type ApplicationService interface {
	ListByJob(jobID string) ([]appmodel.JobApplication, error)
	ListAll(page, limit int) ([]ApplicationWithJob, int, error)
	Create(ctx context.Context, jobID string, input dto.CreateApplicationInput) (*appmodel.JobApplication, error)
	UpdateStage(jobID, appID string, input dto.UpdateStageInput) error
	UpdateNote(jobID, appID string, input dto.UpdateNoteInput) error
}

// ApplicationValidationService validates user-submitted input for the application domain.
type ApplicationValidationService interface {
	ValidateCreate(input dto.CreateApplicationInput) error
	ValidateUpdateStage(input dto.UpdateStageInput) error
	ValidateUpdateNote(input dto.UpdateNoteInput) error
	ValidateListAll(page, limit int) error
}

// ApplicationRepositoryService manages job application stage history.
type ApplicationRepositoryService interface {
	GetApplicationsByJobID(jobResultID string) ([]appmodel.JobApplication, error)
	GetApplicationsByJobIDPaged(jobResultID string, page, limit int) ([]appmodel.JobApplication, int64, error)
	InsertApplication(jobResultID, stage, note string) error
	InsertAppliedApplication(jobResultID string) error
	InsertInterviewingApplication(jobResultID string) error
	InsertOfferApplication(jobResultID string) error
	GetLatestApplicationsWithJob() ([]ApplicationWithJob, error)
	GetLatestApplicationsWithJobPaged(page, limit int) ([]ApplicationWithJob, int64, error)
	UpdateApplicationStage(id int64, stage string) error
	UpdateApplicationNote(id int64, note string) error
}
