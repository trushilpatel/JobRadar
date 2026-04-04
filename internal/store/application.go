package store

import (
	"github.com/trushilpatel/jobradar/internal/model"
	"gorm.io/gorm"
)

// ApplicationStore manages job application stage history.
type ApplicationStore interface {
	GetApplicationsByJobID(jobResultID string) ([]model.JobApplication, error)
	InsertApplication(jobResultID, stage, note string) error
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
