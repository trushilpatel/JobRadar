package store

import (
	"github.com/trushilpatel/jobradar/internal/model"
	"gorm.io/gorm"
)

// JobReader is for the UI/API layer to query enriched jobs.
type JobReader interface {
	GetJobsByStatus(status string) ([]model.Job, error)
	GetJobByID(id string) (*model.Job, error)
}

// JobWriter is for the LLM enricher to insert completed job results.
type JobWriter interface {
	InsertJob(job *model.Job) error
	UpdateJobStatus(id, status string) error
	UpdateJobSaved(id string, saved bool) error
}

// JobStore composes both for contexts that need full access.
type JobStore interface {
	JobReader
	JobWriter
}

type jobStore struct {
	db *gorm.DB
}

func NewJobStore(db *gorm.DB) JobStore {
	return &jobStore{db: db}
}

func (s *jobStore) GetJobsByStatus(status string) ([]model.Job, error) {
	var jobs []model.Job
	err := s.db.Where("status = ?", status).
		Order("added_at DESC").
		Find(&jobs).Error
	return jobs, err
}

func (s *jobStore) GetJobByID(id string) (*model.Job, error) {
	var job model.Job
	err := s.db.Where("id = ?", id).First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *jobStore) InsertJob(job *model.Job) error {
	return s.db.Create(job).Error
}

func (s *jobStore) UpdateJobStatus(id, status string) error {
	return s.db.Model(&model.Job{}).
		Where("id = ?", id).
		Update("status", status).Error
}

func (s *jobStore) UpdateJobSaved(id string, saved bool) error {
	return s.db.Model(&model.Job{}).
		Where("id = ?", id).
		Update("saved", saved).Error
}
