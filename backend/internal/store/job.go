package store

import (
	"github.com/trushilpatel/jobradar/internal/model"
	"gorm.io/gorm"
)

// DailyJobCount holds the number of jobs added on a given date.
type DailyJobCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// JobReader is for the UI/API layer to query enriched jobs.
type JobReader interface {
	GetJobsByStatus(status string) ([]model.Job, error)
	GetJobByID(id string) (*model.Job, error)
	GetJobCounts() (newCount, savedCount, appliedCount int64, err error)
	GetJobsAddedByDay(days int) ([]DailyJobCount, error)
}

// JobWriter is for the LLM enricher to insert completed job results.
type JobWriter interface {
	InsertJob(job *model.Job) error
	UpdateJobStatus(id, status string) error
	UpdateJobSaved(id string, saved bool) error
	UpdateJobNotes(id, notes string) error
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
	query := s.db.Order("added_at DESC")
	// "saved" tab = all jobs where saved=true, regardless of exact status value
	if status == model.JobStatusSaved {
		query = query.Where("saved = ?", true)
	} else {
		query = query.Where("status = ?", status)
	}
	err := query.Find(&jobs).Error
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
	if saved {
		// Advance status to 'saved' unless the job is already applied
		return s.db.Model(&model.Job{}).
			Where("id = ? AND status != ?", id, model.JobStatusApplied).
			Updates(map[string]any{"saved": true, "status": model.JobStatusSaved}).Error
	}
	// Revert status to 'new' only if currently 'saved'; don't touch applied jobs
	return s.db.Exec(
		`UPDATE job SET saved = false, status = CASE WHEN status = ? THEN ? ELSE status END WHERE id = ?`,
		model.JobStatusSaved, model.JobStatusNew, id,
	).Error
}

func (s *jobStore) UpdateJobNotes(id, notes string) error {
	return s.db.Model(&model.Job{}).
		Where("id = ?", id).
		Update("notes", notes).Error
}

func (s *jobStore) GetJobCounts() (newCount, savedCount, appliedCount int64, err error) {
	if err = s.db.Model(&model.Job{}).Where("status = ?", model.JobStatusNew).Count(&newCount).Error; err != nil {
		return
	}
	if err = s.db.Model(&model.Job{}).Where("status = ?", model.JobStatusSaved).Count(&savedCount).Error; err != nil {
		return
	}
	err = s.db.Model(&model.Job{}).Where("status = ?", model.JobStatusApplied).Count(&appliedCount).Error
	return
}

func (s *jobStore) GetJobsAddedByDay(days int) ([]DailyJobCount, error) {
	var results []DailyJobCount
	err := s.db.Model(&model.Job{}).
		Select("DATE(added_at) as date, COUNT(*) as count").
		Where("added_at >= NOW() - INTERVAL ? * INTERVAL '1 day'", days).
		Group("DATE(added_at)").
		Order("date ASC").
		Scan(&results).Error
	return results, err
}
