package services

import (
	"context"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// implService is the terminal ApplicationService implementation. It pulls
// the caller's userID from ctx, passes it explicitly to the repository,
// and orchestrates the cross-domain "verify the job exists" check via the
// JobService. Empty userID fails fast with errNoUser() — a forgotten ctx
// scoping must never silently broaden a query to all users.
type implService struct {
	repo   ApplicationRepository
	jobSvc jobsvc.JobService
}

// NewApplicationImplService constructs the terminal ApplicationService
// chain link, composed over the per-user repository and the job service.
func NewApplicationImplService(repo ApplicationRepository, jobSvc jobsvc.JobService) ApplicationService {
	if repo == nil {
		panic("NewApplicationImplService: repo must not be nil — check wire setup")
	}
	if jobSvc == nil {
		panic("NewApplicationImplService: jobSvc must not be nil — check wire setup")
	}
	return &implService{repo: repo, jobSvc: jobSvc}
}

// ListByJob returns the per-user history of applications against a job.
func (s *implService) ListByJob(ctx context.Context, jobID string) (_ []appmodel.JobApplication, err error) {
	ctx, span := tracing.Start(ctx, "application.impl.ListByJob")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)
	if userID == "" {
		return nil, errNoUser()
	}
	return s.repo.ListByJob(ctx, userID, jobID)
}

// ListAll returns the per-user paginated list of latest applications.
func (s *implService) ListAll(ctx context.Context, page, limit int) (_ []ApplicationWithJob, _ int, err error) {
	ctx, span := tracing.Start(ctx, "application.impl.ListAll")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)
	if userID == "" {
		return nil, 0, errNoUser()
	}
	return s.repo.ListAll(ctx, userID, page, limit)
}

// ListActive returns the per-user list of currently-interviewing apps.
func (s *implService) ListActive(ctx context.Context) (_ []ApplicationWithJob, err error) {
	ctx, span := tracing.Start(ctx, "application.impl.ListActive")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)
	if userID == "" {
		return nil, errNoUser()
	}
	return s.repo.ListActive(ctx, userID)
}

// Create verifies the job exists, then inserts a per-user application row.
// Cross-domain side effects (e.g. setting user_job_state.status='applied')
// are intentionally NOT triggered here — the per-user job state is owned
// by the job slice and is updated via its own endpoint to avoid both
// duplicate writes and recursion through JobService.SetStatus → ApplicationWriter.
func (s *implService) Create(ctx context.Context, jobID string, input dto.CreateApplicationInput) (_ *appmodel.JobApplication, err error) {
	ctx, span := tracing.Start(ctx, "application.impl.Create")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)
	if userID == "" {
		return nil, errNoUser()
	}
	if _, gerr := s.jobSvc.Get(ctx, jobID); gerr != nil {
		return nil, gerr
	}
	app, err := s.repo.Create(ctx, userID, jobID, input)
	if err != nil {
		return nil, apperr.ErrInternal.WithMessage("failed to create application")
	}
	return app, nil
}

// UpdateStage updates the stage of a (user, app) row. The jobID URL
// parameter is preserved in the contract for symmetry but the lookup is
// keyed on (user_id, app_id) at the repo layer.
func (s *implService) UpdateStage(ctx context.Context, _, appID string, input dto.UpdateStageInput) (err error) {
	ctx, span := tracing.Start(ctx, "application.impl.UpdateStage")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)
	if userID == "" {
		return errNoUser()
	}
	return s.repo.UpdateStage(ctx, userID, appID, input)
}

// UpdateStageByID is the dashboard interview-tracker shortcut: same as
// UpdateStage but without a jobID URL component.
func (s *implService) UpdateStageByID(ctx context.Context, appID string, input dto.UpdateStageInput) (err error) {
	ctx, span := tracing.Start(ctx, "application.impl.UpdateStageByID")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)
	if userID == "" {
		return errNoUser()
	}
	return s.repo.UpdateStage(ctx, userID, appID, input)
}

// UpdateNote updates the note of a (user, app) row.
func (s *implService) UpdateNote(ctx context.Context, _, appID string, input dto.UpdateNoteInput) (err error) {
	ctx, span := tracing.Start(ctx, "application.impl.UpdateNote")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)
	if userID == "" {
		return errNoUser()
	}
	return s.repo.UpdateNote(ctx, userID, appID, input)
}
