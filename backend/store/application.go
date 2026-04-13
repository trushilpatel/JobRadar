package store

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

// ApplicationStore manages job application stage history.
type ApplicationStore interface {
	GetApplicationsByJobID(jobResultID string) ([]model.JobApplication, error)
	InsertApplication(jobResultID, stage, note string) error
	GetLatestApplicationsWithJob() ([]ApplicationWithJob, error)
	UpdateApplicationStage(id int64, stage string) error
	UpdateApplicationNote(id int64, note string) error
}

type applicationStore struct {
	db *gorm.DB
}

func NewApplicationStore(db *gorm.DB) ApplicationStore {
	return &applicationStore{db: db}
}

func (s *applicationStore) GetApplicationsByJobID(jobResultID string) ([]model.JobApplication, error) {
	var apps []model.JobApplication
	err := s.db.Where("job_result_id = ?", jobResultID).
		Order("stage_at ASC").
		Find(&apps).Error
	return apps, err
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
