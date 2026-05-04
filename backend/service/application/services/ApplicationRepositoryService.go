package services

import (
	"context"
	"strconv"

	"github.com/trushilpatel/jobradar/common/errors"
	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	"gorm.io/gorm"
)

type applicationRepository struct {
	db *gorm.DB
}

// NewApplicationRepositoryService constructs the terminal ApplicationService implementation.
func NewApplicationRepositoryService(db *gorm.DB) ApplicationService {
	return &applicationRepository{db: db}
}

// NewApplicationWriter constructs an ApplicationWriter backed by the repository.
func NewApplicationWriter(db *gorm.DB) jobsvc.ApplicationWriter {
	return &applicationRepository{db: db}
}

// --- ApplicationService interface ---

func (s *applicationRepository) ListByJob(jobID string) ([]appmodel.JobApplication, error) {
	return s.getApplicationsByJobID(jobID)
}

func (s *applicationRepository) ListAll(page, limit int) ([]ApplicationWithJob, int, error) {
	apps, total, err := s.getLatestApplicationsWithJobPaged(page, limit)
	if err != nil {
		return nil, 0, err
	}
	return apps, int(total), nil
}

func (s *applicationRepository) ListActive() ([]ApplicationWithJob, error) {
	var results []ApplicationWithJob
	err := s.db.Raw(`
		SELECT DISTINCT ON (ja.job_result_id)
			ja.*,
			j.title as job_title,
			j.company_id,
			c.name as company_name
		FROM job_application_tracker ja
		JOIN job_feed j ON ja.job_result_id = j.id
		JOIN companies c ON j.company_id = c.id
		WHERE ja.stage != ?
		ORDER BY ja.job_result_id, ja.stage_at DESC
	`, appcommon.StageRejected).Scan(&results).Error
	return results, err
}

func (s *applicationRepository) Create(_ context.Context, jobResultID string, input dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	if err := s.insertApplication(jobResultID, input.Stage, input.Note); err != nil {
		return nil, err
	}
	app := &appmodel.JobApplication{
		JobResultID: jobResultID,
		Stage:       input.Stage,
	}
	if input.Note != "" {
		app.Note = &input.Note
	}
	return app, nil
}

func (s *applicationRepository) UpdateStage(_, appID string, input dto.UpdateStageInput) error {
	id, err := parseAppID(appID)
	if err != nil {
		return err
	}
	return s.updateApplicationStage(id, input.Stage)
}

func (s *applicationRepository) UpdateNote(_, appID string, input dto.UpdateNoteInput) error {
	id, err := parseAppID(appID)
	if err != nil {
		return err
	}
	return s.updateApplicationNote(id, input.Note)
}

// --- ApplicationWriter interface ---

// InsertAppliedApplication inserts an "applied" stage row.
func (s *applicationRepository) InsertAppliedApplication(jobResultID string) error {
	return s.insertApplication(jobResultID, appcommon.StageApplied, "")
}

// InsertInterviewingApplication inserts a "phone_screen" stage row.
func (s *applicationRepository) InsertInterviewingApplication(jobResultID string) error {
	return s.insertApplication(jobResultID, appcommon.StagePhoneScreen, "")
}

// InsertOfferApplication inserts an "offer" stage row.
func (s *applicationRepository) InsertOfferApplication(jobResultID string) error {
	return s.insertApplication(jobResultID, appcommon.StageOffer, "")
}

// --- unexported DB helpers ---

func parseAppID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, errors.ErrBadRequest.WithMessage("invalid app_id")
	}
	return id, nil
}

func (s *applicationRepository) getApplicationsByJobID(jobResultID string) ([]appmodel.JobApplication, error) {
	var apps []appmodel.JobApplication
	err := s.db.Where("job_result_id = ?", jobResultID).
		Order("stage_at ASC").
		Limit(100).
		Find(&apps).Error
	return apps, err
}

func (s *applicationRepository) insertApplication(jobResultID, stage, note string) error {
	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	row := appmodel.JobApplication{
		JobResultID: jobResultID,
		Stage:       stage,
		Note:        notePtr,
	}
	return s.db.Create(&row).Error
}

func (s *applicationRepository) getLatestApplicationsWithJobPaged(page, limit int) ([]ApplicationWithJob, int64, error) {
	var results []ApplicationWithJob
	var total int64

	if err := s.db.Raw(`SELECT COUNT(DISTINCT job_result_id) FROM job_application_tracker`).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	err := s.db.Raw(`
		SELECT DISTINCT ON (ja.job_result_id)
			ja.*,
			j.title as job_title,
			j.company_id,
			c.name as company_name
		FROM job_application_tracker ja
		JOIN job_feed j ON ja.job_result_id = j.id
		JOIN companies c ON j.company_id = c.id
		ORDER BY ja.job_result_id, ja.stage_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset).Scan(&results).Error
	return results, total, err
}

func (s *applicationRepository) updateApplicationStage(id int64, stage string) error {
	return s.db.Exec(
		`UPDATE job_application_tracker SET stage = ?, stage_at = NOW() WHERE id = ?`,
		stage, id,
	).Error
}

func (s *applicationRepository) updateApplicationNote(id int64, note string) error {
	return s.db.Exec(
		`UPDATE job_application_tracker SET note = ? WHERE id = ?`,
		note, id,
	).Error
}
