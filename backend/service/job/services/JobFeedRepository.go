package services

import (
	"context"
	"strings"

	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/model"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
	"gorm.io/gorm"
)

// JobReader is for the UI/API layer to query enriched jobs.
type JobReader interface {
	GetJobs(ctx context.Context, filter dto.JobFilter, page, limit int) ([]model.Job, int64, error)
	GetJobByID(ctx context.Context, id string) (*model.Job, error)
	GetJobCounts(ctx context.Context) (*dto.JobCounts, error)
	GetJobsAddedByDay(ctx context.Context, days int) ([]dto.DailyJobCount, error)
}

// JobWriter is for the LLM enricher to insert completed job results.
type JobWriter interface {
	InsertJob(ctx context.Context, job *model.Job) error
	DeleteJob(ctx context.Context, id string) error
	UpdateJobStatus(ctx context.Context, id, status string) error
	UpdateJobSaved(ctx context.Context, id string, saved bool) error
	UpdateJobNotes(ctx context.Context, id, notes string) error
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

func (s *jobStore) GetJobs(ctx context.Context, filter dto.JobFilter, page, limit int) (_ []model.Job, _ int64, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobs")
	defer tracing.End(span, &err)
	query := s.db.WithContext(ctx).Model(&model.Job{})

	// Status — always applied; "saved" is a boolean flag, not a status value
	if filter.Status == jobcommon.JobStatusSaved {
		query = query.Where("saved = ?", true)
	} else if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	if len(filter.Companies) > 0 {
		query = query.Where("company_id IN ?", filter.Companies)
	}

	if filter.Remote != nil {
		query = query.Where("remote = ?", *filter.Remote)
	}

	if filter.OnSite {
		query = query.Where("remote = ?", false)
	}

	if len(filter.EmployTypes) > 0 {
		query = query.Where("employ_type IN ?", filter.EmployTypes)
	}

	if filter.Sponsorship != nil {
		query = query.Where("sponsorship = ?", *filter.Sponsorship)
	}

	if filter.NoRequirements {
		query = query.Where("requires_citizen = ? AND security_clearance = ?", false, false)
	}

	if filter.RequiresCitizen != nil {
		query = query.Where("requires_citizen = ?", *filter.RequiresCitizen)
	}

	if len(filter.ExperienceRanges) > 0 {
		var clauses []string
		var args []interface{}
		for _, r := range filter.ExperienceRanges {
			if r.Max != nil {
				if r.Min == 0 {
					clauses = append(clauses, "(experience_years <= ? OR experience_years IS NULL)")
					args = append(args, *r.Max)
				} else {
					clauses = append(clauses, "(experience_years > ? AND experience_years <= ?)")
					args = append(args, r.Min, *r.Max)
				}
			} else {
				clauses = append(clauses, "experience_years > ?")
				args = append(args, r.Min)
			}
		}
		if len(clauses) > 0 {
			query = query.Where("("+strings.Join(clauses, " OR ")+")", args...)
		}
	}

	if filter.DaysAdded != nil {
		query = query.Where("added_at >= NOW() - ? * INTERVAL '1 day'", *filter.DaysAdded)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var jobs []model.Job
	offset := (page - 1) * limit
	err = query.Order("added_at DESC").Offset(offset).Limit(limit).Preload("Company").Find(&jobs).Error
	return jobs, total, err
}

func (s *jobStore) GetJobByID(ctx context.Context, id string) (_ *model.Job, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobByID")
	defer tracing.End(span, &err)
	var job model.Job
	err = s.db.WithContext(ctx).Preload("Company").Where("id = ?", id).First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *jobStore) InsertJob(ctx context.Context, job *model.Job) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertJob")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Create(job).Error
}

func (s *jobStore) DeleteJob(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.DeleteJob")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Job{}).Error
}

func (s *jobStore) UpdateJobStatus(ctx context.Context, id, status string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateJobStatus")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Model(&model.Job{}).
		Where("id = ?", id).
		Update("status", status).Error
}

func (s *jobStore) UpdateJobSaved(ctx context.Context, id string, saved bool) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateJobSaved")
	defer tracing.End(span, &err)
	if saved {
		return s.db.WithContext(ctx).Model(&model.Job{}).
			Where("id = ? AND status != ?", id, jobcommon.JobStatusApplied).
			Updates(map[string]any{"saved": true, "status": jobcommon.JobStatusSaved}).Error
	}
	return s.db.WithContext(ctx).Exec(
		`UPDATE job_feed SET saved = false, status = CASE WHEN status = ? THEN ? ELSE status END WHERE id = ?`,
		jobcommon.JobStatusSaved, jobcommon.JobStatusNew, id,
	).Error
}

func (s *jobStore) UpdateJobNotes(ctx context.Context, id, notes string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateJobNotes")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Model(&model.Job{}).
		Where("id = ?", id).
		Update("notes", notes).Error
}

func (s *jobStore) GetJobCounts(ctx context.Context) (_ *dto.JobCounts, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobCounts")
	defer tracing.End(span, &err)
	var rows []dto.CountsResult
	err = s.db.WithContext(ctx).Raw(`
		SELECT status, COUNT(*) as count
		FROM job_feed
		WHERE status IN (?, ?, ?, ?, ?)
		GROUP BY status
	`,
		jobcommon.JobStatusNew,
		jobcommon.JobStatusSaved,
		jobcommon.JobStatusApplied,
		jobcommon.JobStatusInterviewing,
		jobcommon.JobStatusOffer,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	counts := &dto.JobCounts{}
	for _, r := range rows {
		switch r.Status {
		case jobcommon.JobStatusNew:
			counts.New = r.Count
		case jobcommon.JobStatusSaved:
			counts.Saved = r.Count
		case jobcommon.JobStatusApplied:
			counts.Applied = r.Count
		case jobcommon.JobStatusInterviewing:
			counts.Interviewing = r.Count
		case jobcommon.JobStatusOffer:
			counts.Offer = r.Count
		}
	}
	return counts, nil
}

func (s *jobStore) GetJobsAddedByDay(ctx context.Context, days int) (results []dto.DailyJobCount, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobsAddedByDay")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Model(&model.Job{}).
		Select("DATE(added_at) as date, COUNT(*) as count").
		Where("added_at >= NOW() - ? * INTERVAL '1 day'", days).
		Group("DATE(added_at)").
		Order("date ASC").
		Scan(&results).Error
	return results, err
}
