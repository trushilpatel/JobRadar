package application

import (
	"strconv"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	jobsvc "github.com/trushilpatel/jobradar/service/job"
)

// Service defines all business operations for the application domain.
type Service interface {
	ListByJob(jobID string) ([]model.JobApplication, error)
	ListAll(page, limit int) ([]ApplicationWithJob, int, error)
	Create(jobID string, input CreateApplicationInput) (*model.JobApplication, error)
	UpdateStage(jobID, appID string, input UpdateStageInput) error
	UpdateNote(jobID, appID string, input UpdateNoteInput) error
}

type service struct {
	store  Store
	jobSvc jobsvc.Service
}

// NewService constructs a Service implementation.
func NewService(s Store, jobSvc jobsvc.Service) Service {
	return &service{store: s, jobSvc: jobSvc}
}

// ListByJob returns all application entries for a job.
func (s *service) ListByJob(jobID string) ([]model.JobApplication, error) {
	apps, err := s.store.GetApplicationsByJobID(jobID)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch applications")
	}
	return apps, nil
}

// ListAll returns a paginated list of the latest application per job.
func (s *service) ListAll(page, limit int) ([]ApplicationWithJob, int, error) {
	apps, total, err := s.store.GetLatestApplicationsWithJobPaged(page, limit)
	if err != nil {
		return nil, 0, domain.ErrInternal.WithMessage("failed to fetch applications")
	}
	return apps, int(total), nil
}

// Create inserts a new application stage entry for the given job.
// Verifies the job exists via job.Service.Get().
func (s *service) Create(jobID string, input CreateApplicationInput) (*model.JobApplication, error) {
	// Existence check — returns NOT_FOUND if missing
	if _, err := s.jobSvc.Get(jobID); err != nil {
		return nil, err
	}

	if err := s.store.InsertApplication(jobID, input.Stage, input.Note); err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to create application")
	}

	// When the first stage is added, mark the job as applied (non-fatal)
	if input.Stage == model.StageApplied {
		_ = s.jobSvc.SetStatus(jobID, model.JobStatusApplied)
	}

	app := &model.JobApplication{
		JobResultID: jobID,
		Stage:       input.Stage,
	}
	if input.Note != "" {
		app.Note = &input.Note
	}
	return app, nil
}

// UpdateStage updates the stage of an existing application entry.
func (s *service) UpdateStage(jobID, appID string, input UpdateStageInput) error {
	id, err := strconv.ParseInt(appID, 10, 64)
	if err != nil {
		return domain.ErrBadRequest.WithMessage("invalid app_id")
	}
	if err := s.store.UpdateApplicationStage(id, input.Stage); err != nil {
		return domain.ErrInternal.WithMessage("failed to update stage")
	}
	return nil
}

// UpdateNote updates the note of an existing application entry.
func (s *service) UpdateNote(jobID, appID string, input UpdateNoteInput) error {
	id, err := strconv.ParseInt(appID, 10, 64)
	if err != nil {
		return domain.ErrBadRequest.WithMessage("invalid app_id")
	}
	if err := s.store.UpdateApplicationNote(id, input.Note); err != nil {
		return domain.ErrInternal.WithMessage("failed to update note")
	}
	return nil
}
