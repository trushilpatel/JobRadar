package services

import (
	"context"
	"strconv"

	"github.com/trushilpatel/jobradar/domain"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

type applicationImpl struct {
	repo   ApplicationRepositoryService
	jobSvc jobsvc.Service
}

// NewApplicationImplService constructs an ApplicationService implementation.
func NewApplicationImplService(repo ApplicationRepositoryService, jobSvc jobsvc.Service) ApplicationService {
	return &applicationImpl{repo: repo, jobSvc: jobSvc}
}

// ListByJob returns all application entries for a job.
func (s *applicationImpl) ListByJob(jobID string) ([]appmodel.JobApplication, error) {
	apps, err := s.repo.GetApplicationsByJobID(jobID)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch applications")
	}
	return apps, nil
}

// ListAll returns a paginated list of the latest application per job.
func (s *applicationImpl) ListAll(page, limit int) ([]ApplicationWithJob, int, error) {
	apps, total, err := s.repo.GetLatestApplicationsWithJobPaged(page, limit)
	if err != nil {
		return nil, 0, domain.ErrInternal.WithMessage("failed to fetch applications")
	}
	return apps, int(total), nil
}

// Create inserts a new application stage entry for the given job.
// Verifies the job exists via job.Service.Get().
func (s *applicationImpl) Create(ctx context.Context, jobID string, input dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	// Existence check — returns NOT_FOUND if missing
	if _, err := s.jobSvc.Get(ctx, jobID); err != nil {
		return nil, err
	}

	if err := s.repo.InsertApplication(jobID, input.Stage, input.Note); err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to create application")
	}

	// When the first stage is added, mark the job as applied (non-fatal)
	if input.Stage == appcommon.StageApplied {
		_ = s.jobSvc.MarkApplied(ctx, jobID)
	}

	app := &appmodel.JobApplication{
		JobResultID: jobID,
		Stage:       input.Stage,
	}
	if input.Note != "" {
		app.Note = &input.Note
	}
	return app, nil
}

// UpdateStage updates the stage of an existing application entry.
func (s *applicationImpl) UpdateStage(jobID, appID string, input dto.UpdateStageInput) error {
	id, err := strconv.ParseInt(appID, 10, 64)
	if err != nil {
		return domain.ErrBadRequest.WithMessage("invalid app_id")
	}
	if err := s.repo.UpdateApplicationStage(id, input.Stage); err != nil {
		return domain.ErrInternal.WithMessage("failed to update stage")
	}
	return nil
}

// UpdateNote updates the note of an existing application entry.
func (s *applicationImpl) UpdateNote(jobID, appID string, input dto.UpdateNoteInput) error {
	id, err := strconv.ParseInt(appID, 10, 64)
	if err != nil {
		return domain.ErrBadRequest.WithMessage("invalid app_id")
	}
	if err := s.repo.UpdateApplicationNote(id, input.Note); err != nil {
		return domain.ErrInternal.WithMessage("failed to update note")
	}
	return nil
}
