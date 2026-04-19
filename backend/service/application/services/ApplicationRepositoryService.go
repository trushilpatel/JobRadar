package services

import (
	"github.com/trushilpatel/jobradar/model"
	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"gorm.io/gorm"
)

type applicationRepository struct {
	db *gorm.DB
}

// NewApplicationRepositoryService constructs an ApplicationRepositoryService implementation.
func NewApplicationRepositoryService(db *gorm.DB) ApplicationRepositoryService {
	return &applicationRepository{db: db}
}

func (s *applicationRepository) GetApplicationsByJobID(jobResultID string) ([]model.JobApplication, error) {
	var apps []model.JobApplication
	err := s.db.Where("job_result_id = ?", jobResultID).
		Order("stage_at ASC").
		Find(&apps).Error
	return apps, err
}

func (s *applicationRepository) GetApplicationsByJobIDPaged(jobResultID string, page, limit int) ([]model.JobApplication, int64, error) {
	var apps []model.JobApplication
	var total int64
	query := s.db.Model(&model.JobApplication{}).Where("job_result_id = ?", jobResultID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	err := query.Order("stage_at ASC").Offset(offset).Limit(limit).Find(&apps).Error
	return apps, total, err
}

func (s *applicationRepository) InsertApplication(jobResultID, stage, note string) error {
	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	row := model.JobApplication{
		JobResultID: jobResultID,
		Stage:       stage,
		Note:        notePtr,
	}
	return s.db.Create(&row).Error
}

func (s *applicationRepository) GetLatestApplicationsWithJob() ([]ApplicationWithJob, error) {
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
		ORDER BY ja.job_result_id, ja.stage_at DESC
	`).Scan(&results).Error
	return results, err
}

func (s *applicationRepository) GetLatestApplicationsWithJobPaged(page, limit int) ([]ApplicationWithJob, int64, error) {
	var results []ApplicationWithJob
	var total int64

	countErr := s.db.Raw(`
		SELECT COUNT(DISTINCT job_result_id) FROM job_application_tracker
	`).Scan(&total).Error
	if countErr != nil {
		return nil, 0, countErr
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

// InsertAppliedApplication implements job.ApplicationWriter — inserts an "applied" stage row.
func (s *applicationRepository) InsertAppliedApplication(jobResultID string) error {
	return s.InsertApplication(jobResultID, appcommon.StageApplied, "")
}

// InsertInterviewingApplication implements job.ApplicationWriter — inserts a "phone_screen" stage row.
func (s *applicationRepository) InsertInterviewingApplication(jobResultID string) error {
	return s.InsertApplication(jobResultID, appcommon.StagePhoneScreen, "")
}

// InsertOfferApplication implements job.ApplicationWriter — inserts an "offer" stage row.
func (s *applicationRepository) InsertOfferApplication(jobResultID string) error {
	return s.InsertApplication(jobResultID, appcommon.StageOffer, "")
}

func (s *applicationRepository) UpdateApplicationStage(id int64, stage string) error {
	return s.db.Exec(
		`UPDATE job_application_tracker SET stage = ?, stage_at = NOW() WHERE id = ?`,
		stage, id,
	).Error
}

func (s *applicationRepository) UpdateApplicationNote(id int64, note string) error {
	return s.db.Exec(
		`UPDATE job_application_tracker SET note = ? WHERE id = ?`,
		note, id,
	).Error
}
