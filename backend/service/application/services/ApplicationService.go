package services

import (
	"context"

	"github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
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
	ListActive() ([]ApplicationWithJob, error)
	Create(ctx context.Context, jobID string, input dto.CreateApplicationInput) (*appmodel.JobApplication, error)
	UpdateStage(jobID, appID string, input dto.UpdateStageInput) error
	UpdateNote(jobID, appID string, input dto.UpdateNoteInput) error
}

// BaseService auto-delegates all ApplicationService methods to next.
// Embed this in layers that only need to override a subset of methods.
type BaseService struct {
	next ApplicationService
}

// NewBaseService constructs a BaseService wrapping next. Panics if next is nil.
func NewBaseService(next ApplicationService) BaseService {
	if next == nil {
		panic("BaseService: next must not be nil — check wire setup")
	}
	return BaseService{next: next}
}

func (b *BaseService) ListByJob(jobID string) ([]appmodel.JobApplication, error) {
	return b.next.ListByJob(jobID)
}

func (b *BaseService) ListAll(page, limit int) ([]ApplicationWithJob, int, error) {
	return b.next.ListAll(page, limit)
}

func (b *BaseService) ListActive() ([]ApplicationWithJob, error) {
	return b.next.ListActive()
}

func (b *BaseService) Create(ctx context.Context, jobID string, input dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	return b.next.Create(ctx, jobID, input)
}

func (b *BaseService) UpdateStage(jobID, appID string, input dto.UpdateStageInput) error {
	return b.next.UpdateStage(jobID, appID, input)
}

func (b *BaseService) UpdateNote(jobID, appID string, input dto.UpdateNoteInput) error {
	return b.next.UpdateNote(jobID, appID, input)
}
