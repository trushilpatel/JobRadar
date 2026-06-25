package services

import (
	"context"
	"strconv"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	"github.com/trushilpatel/jobradar/common/tracing"
	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	"gorm.io/gorm"
)

var _ ApplicationRepository = (*applicationRepository)(nil)

// applicationRepository is the GORM-backed terminal ApplicationRepository
// implementation. Every method takes userID explicitly; an empty userID is
// rejected with ErrInternal.WithMessage("user id required") before the DB
// is touched, so a forgotten ctx scoping never silently broadens a query
// to all users.
type applicationRepository struct {
	db *gorm.DB
}

// NewApplicationRepositoryService constructs the per-user ApplicationRepository.
func NewApplicationRepositoryService(db *gorm.DB) ApplicationRepository {
	return &applicationRepository{db: db}
}

// NewApplicationWriter constructs the narrow jobsvc.ApplicationWriter
// contract backed by the same repository. The caller's userID is sourced
// from ctx in InsertAppliedApplication.
func NewApplicationWriter(db *gorm.DB) jobsvc.ApplicationWriter {
	return &applicationRepository{db: db}
}

// errNoUser is the canonical guard error for empty userID. The same error
// instance is returned from every method so AssertUserScoped tests have a
// stable check.
func errNoUser() error {
	return apperr.ErrInternal.WithMessage("user id required")
}

// --- ApplicationRepository interface ---

// ListByJob returns the application history for one (user, job_result_id) pair.
func (s *applicationRepository) ListByJob(ctx context.Context, userID, jobID string) (_ []appmodel.JobApplication, err error) {
	if userID == "" {
		return nil, errNoUser()
	}
	ctx, span := tracing.Start(ctx, "application.repo.ListByJob")
	defer tracing.End(span, &err)

	var apps []appmodel.JobApplication
	err = s.db.WithContext(ctx).
		Where("user_id = ? AND job_result_id = ?", userID, jobID).
		Order("stage_at ASC").
		Limit(100).
		Find(&apps).Error
	return apps, err
}

// ListAll returns a paginated list of the latest application per
// (user, job_result_id) belonging to userID.
func (s *applicationRepository) ListAll(ctx context.Context, userID string, page, limit int) (_ []ApplicationWithJob, _ int, err error) {
	if userID == "" {
		return nil, 0, errNoUser()
	}
	ctx, span := tracing.Start(ctx, "application.repo.ListAll")
	defer tracing.End(span, &err)

	var results []ApplicationWithJob
	var total int64

	if err = s.db.WithContext(ctx).
		Raw(`SELECT COUNT(DISTINCT job_result_id) FROM job_application_tracker WHERE user_id = ?`, userID).
		Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	err = s.db.WithContext(ctx).Raw(`
		SELECT DISTINCT ON (ja.job_result_id)
			ja.*,
			j.title as job_title,
			j.company_id,
			c.name as company_name
		FROM job_application_tracker ja
		JOIN jobs j ON ja.job_result_id = j.id
		JOIN companies c ON j.company_id = c.id
		WHERE ja.user_id = ?
		ORDER BY ja.job_result_id, ja.stage_at DESC
		LIMIT ? OFFSET ?
	`, userID, limit, offset).Scan(&results).Error
	return results, int(total), err
}

// ListActive returns the latest application per (user, job_result_id) for
// any application currently in an interview-track stage.
func (s *applicationRepository) ListActive(ctx context.Context, userID string) (_ []ApplicationWithJob, err error) {
	if userID == "" {
		return nil, errNoUser()
	}
	ctx, span := tracing.Start(ctx, "application.repo.ListActive")
	defer tracing.End(span, &err)

	var results []ApplicationWithJob
	interviewingStages := []string{
		appcommon.StagePhoneScreen,
		appcommon.StageTechnical,
		appcommon.StageFinalRound,
		appcommon.StageOffer,
	}
	err = s.db.WithContext(ctx).Raw(`
		SELECT DISTINCT ON (ja.job_result_id)
			ja.*,
			j.title as job_title,
			j.company_id,
			c.name as company_name
		FROM job_application_tracker ja
		JOIN jobs j ON ja.job_result_id = j.id
		JOIN companies c ON j.company_id = c.id
		WHERE ja.user_id = ? AND ja.stage IN ?
		ORDER BY ja.job_result_id, ja.stage_at DESC
	`, userID, interviewingStages).Scan(&results).Error
	return results, err
}

// Create inserts a new application row scoped to (user, jobResultID).
func (s *applicationRepository) Create(ctx context.Context, userID, jobResultID string, input dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	if userID == "" {
		return nil, errNoUser()
	}
	return s.insertApplication(ctx, userID, jobResultID, input.Stage, input.Note)
}

// UpdateStage updates the stage of an application owned by userID.
// Returns ErrNotFound when no row matches (wrong ID or cross-user attempt).
func (s *applicationRepository) UpdateStage(ctx context.Context, userID, appID string, input dto.UpdateStageInput) (err error) {
	if userID == "" {
		return errNoUser()
	}
	id, err := parseAppID(appID)
	if err != nil {
		return err
	}
	ctx, span := tracing.Start(ctx, "application.repo.UpdateStage")
	defer tracing.End(span, &err)

	res := s.db.WithContext(ctx).Exec(
		`UPDATE job_application_tracker SET stage = ?, stage_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`,
		input.Stage, id, userID,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperr.ErrNotFound.WithMessage("application not found")
	}
	return nil
}

// UpdateNote updates the note of an application owned by userID.
// Returns ErrNotFound when no row matches (wrong ID or cross-user attempt).
func (s *applicationRepository) UpdateNote(ctx context.Context, userID, appID string, input dto.UpdateNoteInput) (err error) {
	if userID == "" {
		return errNoUser()
	}
	id, err := parseAppID(appID)
	if err != nil {
		return err
	}
	ctx, span := tracing.Start(ctx, "application.repo.UpdateNote")
	defer tracing.End(span, &err)

	res := s.db.WithContext(ctx).Exec(
		`UPDATE job_application_tracker SET note = ? WHERE id = ? AND user_id = ?`,
		input.Note, id, userID,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperr.ErrNotFound.WithMessage("application not found")
	}
	return nil
}

// --- jobsvc.ApplicationWriter interface ---

// InsertAppliedApplication inserts an "applied" stage row for the caller
// in ctx. The userID is sourced from ctx (per the ApplicationWriter
// contract) — empty userID is rejected with errNoUser().
func (s *applicationRepository) InsertAppliedApplication(ctx context.Context, jobResultID string) error {
	userID := identity.UserIDFrom(ctx)
	if userID == "" {
		return errNoUser()
	}
	_, err := s.insertApplication(ctx, userID, jobResultID, appcommon.StageApplied, "")
	return err
}

// --- unexported helpers ---

func parseAppID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, apperr.ErrBadRequest.WithMessage("invalid app_id")
	}
	return id, nil
}

func (s *applicationRepository) insertApplication(ctx context.Context, userID, jobResultID, stage, note string) (_ *appmodel.JobApplication, err error) {
	ctx, span := tracing.Start(ctx, "application.repo.insertApplication")
	defer tracing.End(span, &err)

	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	row := appmodel.JobApplication{
		UserID:      userID,
		JobResultID: jobResultID,
		Stage:       stage,
		Note:        notePtr,
	}
	if err = s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
