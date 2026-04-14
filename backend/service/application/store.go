package application

import (
	"github.com/trushilpatel/jobradar/model"
	"gorm.io/gorm"
)

// ApplicationWithJob joins a JobApplication with its Job title and Company name.
type ApplicationWithJob struct {
	model.JobApplication
	JobTitle    string `json:"job_title"`
	CompanyID   string `json:"company_id"`
	CompanyName string `json:"company_name"`
}

// Store manages job application stage history.
type Store interface {
	GetApplicationsByJobID(jobResultID string) ([]model.JobApplication, error)
	GetApplicationsByJobIDPaged(jobResultID string, page, limit int) ([]model.JobApplication, int64, error)
	InsertApplication(jobResultID, stage, note string) error
	GetLatestApplicationsWithJob() ([]ApplicationWithJob, error)
	GetLatestApplicationsWithJobPaged(page, limit int) ([]ApplicationWithJob, int64, error)
	UpdateApplicationStage(id int64, stage string) error
	UpdateApplicationNote(id int64, note string) error
}

type applicationStore struct {
	db *gorm.DB
}

// NewStore constructs a Store implementation.
func NewStore(db *gorm.DB) Store {
	return &applicationStore{db: db}
}

func (s *applicationStore) GetApplicationsByJobID(jobResultID string) ([]model.JobApplication, error) {
	var apps []model.JobApplication
	err := s.db.Where("job_result_id = ?", jobResultID).
		Order("stage_at ASC").
		Find(&apps).Error
	return apps, err
}

func (s *applicationStore) GetApplicationsByJobIDPaged(jobResultID string, page, limit int) ([]model.JobApplication, int64, error) {
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

func (s *applicationStore) InsertApplication(jobResultID, stage, note string) error {
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

func (s *applicationStore) GetLatestApplicationsWithJob() ([]ApplicationWithJob, error) {
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

func (s *applicationStore) GetLatestApplicationsWithJobPaged(page, limit int) ([]ApplicationWithJob, int64, error) {
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

func (s *applicationStore) UpdateApplicationStage(id int64, stage string) error {
	return s.db.Exec(
		`UPDATE job_application_tracker SET stage = ?, stage_at = NOW() WHERE id = ?`,
		stage, id,
	).Error
}

func (s *applicationStore) UpdateApplicationNote(id int64, note string) error {
	return s.db.Exec(
		`UPDATE job_application_tracker SET note = ? WHERE id = ?`,
		note, id,
	).Error
}
