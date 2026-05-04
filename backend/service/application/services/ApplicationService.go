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

// ApplicationService is the single user-facing contract for the application
// domain. Every method takes ctx; service layers derive the caller's userID
// from ctx via identity.UserIDFrom and pass it explicitly to the repository
// — this guarantees a forgotten ctx scoping cannot silently broaden a query
// to all users.
type ApplicationService interface {
	ListByJob(ctx context.Context, jobID string) ([]appmodel.JobApplication, error)
	ListAll(ctx context.Context, page, limit int) ([]ApplicationWithJob, int, error)
	ListActive(ctx context.Context) ([]ApplicationWithJob, error)
	Create(ctx context.Context, jobID string, input dto.CreateApplicationInput) (*appmodel.JobApplication, error)
	UpdateStage(ctx context.Context, jobID, appID string, input dto.UpdateStageInput) error
	UpdateStageByID(ctx context.Context, appID string, input dto.UpdateStageInput) error
	UpdateNote(ctx context.Context, jobID, appID string, input dto.UpdateNoteInput) error
}

// ApplicationRepository is the per-user repository contract. Every method
// takes userID as an explicit parameter — empty userID is rejected with
// ErrInternal.WithMessage("user id required") so a forgotten ctx scoping
// never silently broadens a query to all users.
type ApplicationRepository interface {
	ListByJob(ctx context.Context, userID, jobID string) ([]appmodel.JobApplication, error)
	ListAll(ctx context.Context, userID string, page, limit int) ([]ApplicationWithJob, int, error)
	ListActive(ctx context.Context, userID string) ([]ApplicationWithJob, error)
	Create(ctx context.Context, userID, jobID string, input dto.CreateApplicationInput) (*appmodel.JobApplication, error)
	UpdateStage(ctx context.Context, userID, appID string, input dto.UpdateStageInput) error
	UpdateNote(ctx context.Context, userID, appID string, input dto.UpdateNoteInput) error
}

// BaseService auto-delegates all ApplicationService methods to next.
// Embed this in layers that only need to override a subset of methods.
type BaseService struct {
	next ApplicationService
}

// NewBaseService constructs a BaseService wrapping next. Panics if next is
// nil — a nil next is always a wiring error, not a runtime condition.
func NewBaseService(next ApplicationService) BaseService {
	if next == nil {
		panic("BaseService: next must not be nil — check wire setup")
	}
	return BaseService{next: next}
}

func (b *BaseService) ListByJob(ctx context.Context, jobID string) ([]appmodel.JobApplication, error) {
	return b.next.ListByJob(ctx, jobID)
}

func (b *BaseService) ListAll(ctx context.Context, page, limit int) ([]ApplicationWithJob, int, error) {
	return b.next.ListAll(ctx, page, limit)
}

func (b *BaseService) ListActive(ctx context.Context) ([]ApplicationWithJob, error) {
	return b.next.ListActive(ctx)
}

func (b *BaseService) Create(ctx context.Context, jobID string, input dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	return b.next.Create(ctx, jobID, input)
}

func (b *BaseService) UpdateStage(ctx context.Context, jobID, appID string, input dto.UpdateStageInput) error {
	return b.next.UpdateStage(ctx, jobID, appID, input)
}

func (b *BaseService) UpdateStageByID(ctx context.Context, appID string, input dto.UpdateStageInput) error {
	return b.next.UpdateStageByID(ctx, appID, input)
}

func (b *BaseService) UpdateNote(ctx context.Context, jobID, appID string, input dto.UpdateNoteInput) error {
	return b.next.UpdateNote(ctx, jobID, appID, input)
}
