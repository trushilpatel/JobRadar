package services

import (
	"context"
	"strings"

	"github.com/trushilpatel/jobradar/common/domain"
	"github.com/trushilpatel/jobradar/platform/tracing"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/gorm"
)

// JobReader is for the UI/API layer to query enriched jobs.
type JobReader interface {
	GetJobs(ctx context.Context, filter dto.JobFilter, page, limit int) ([]jobmodel.Job, int64, error)
	GetJobByID(ctx context.Context, id string) (*jobmodel.Job, error)
	GetJobCounts(ctx context.Context) (*dto.JobCounts, error)
	GetJobsAddedByDay(ctx context.Context, days int) ([]dto.DailyJobCount, error)
}

// JobWriter is for the LLM enricher to insert completed job results.
type JobWriter interface {
	InsertJob(ctx context.Context, job *jobmodel.Job) error
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

func (s *jobStore) GetJobs(ctx context.Context, filter dto.JobFilter, page, limit int) (_ []jobmodel.Job, _ int64, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobs")
	defer tracing.End(span, &err)
	query := s.db.WithContext(ctx).Model(&jobmodel.Job{})

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
		var args []any
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

	var jobs []jobmodel.Job
	offset := (page - 1) * limit
	err = query.Order("added_at DESC").Offset(offset).Limit(limit).Preload("Company").Find(&jobs).Error
	return jobs, total, err
}

func (s *jobStore) GetJobByID(ctx context.Context, id string) (_ *jobmodel.Job, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobByID")
	defer tracing.End(span, &err)
	var job jobmodel.Job
	err = s.db.WithContext(ctx).Preload("Company").Where("id = ?", id).First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *jobStore) InsertJob(ctx context.Context, job *jobmodel.Job) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertJob")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Create(job).Error
}

func (s *jobStore) DeleteJob(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.DeleteJob")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Where("id = ?", id).Delete(&jobmodel.Job{}).Error
}

func (s *jobStore) UpdateJobStatus(ctx context.Context, id, status string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateJobStatus")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Model(&jobmodel.Job{}).
		Where("id = ?", id).
		Update("status", status).Error
}

func (s *jobStore) UpdateJobSaved(ctx context.Context, id string, saved bool) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateJobSaved")
	defer tracing.End(span, &err)
	if saved {
		return s.db.WithContext(ctx).Model(&jobmodel.Job{}).
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
	return s.db.WithContext(ctx).Model(&jobmodel.Job{}).
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
	err = s.db.WithContext(ctx).Model(&jobmodel.Job{}).
		Select("DATE(added_at) as date, COUNT(*) as count").
		Where("added_at >= NOW() - ? * INTERVAL '1 day'", days).
		Group("DATE(added_at)").
		Order("date ASC").
		Scan(&results).Error
	return results, err
}

// repositoryService is the terminal layer in the job Service decorator chain.
// It implements all Service methods by delegating directly to the JobStore.
type repositoryService struct {
	store JobStore
}

// NewRepositoryService constructs the terminal Service backed by a JobStore.
func NewRepositoryService(s JobStore) Service {
	return &repositoryService{store: s}
}

func (r *repositoryService) List(ctx context.Context, filter dto.JobFilter, page, limit int) (_ []jobmodel.Job, _ int, err error) {
	ctx, span := tracing.Start(ctx, "job.repository.List")
	defer tracing.End(span, &err)
	jobs, total, err := r.store.GetJobs(ctx, filter, page, limit)
	if err != nil {
		return nil, 0, domain.ErrInternal.WithMessage("failed to fetch jobs")
	}
	return jobs, int(total), nil
}

func (r *repositoryService) Get(ctx context.Context, id string) (_ *jobmodel.Job, err error) {
	ctx, span := tracing.Start(ctx, "job.repository.Get")
	defer tracing.End(span, &err)
	job, err := r.store.GetJobByID(ctx, id)
	if err != nil {
		if domain.IsNotFound(err) {
			return nil, domain.ErrNotFound.WithMessage("job not found")
		}
		return nil, domain.ErrInternal.WithMessage("failed to fetch job")
	}
	return job, nil
}

func (r *repositoryService) GetCounts(ctx context.Context) (_ *dto.JobCounts, err error) {
	ctx, span := tracing.Start(ctx, "job.repository.GetCounts")
	defer tracing.End(span, &err)
	counts, err := r.store.GetJobCounts(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch job counts")
	}
	return counts, nil
}

func (r *repositoryService) Dismiss(ctx context.Context, id string, _ dto.DismissInput) (err error) {
	ctx, span := tracing.Start(ctx, "job.repository.Dismiss")
	defer tracing.End(span, &err)
	if err := r.store.UpdateJobStatus(ctx, id, jobcommon.JobStatusDismissed); err != nil {
		return domain.ErrInternal.WithMessage("failed to update job status")
	}
	return nil
}

func (r *repositoryService) SetSaved(ctx context.Context, id string, saved bool) (err error) {
	ctx, span := tracing.Start(ctx, "job.repository.SetSaved")
	defer tracing.End(span, &err)
	if err := r.store.UpdateJobSaved(ctx, id, saved); err != nil {
		return domain.ErrInternal.WithMessage("failed to update job")
	}
	return nil
}

func (r *repositoryService) SetStatus(ctx context.Context, id, status string) (err error) {
	ctx, span := tracing.Start(ctx, "job.repository.SetStatus")
	defer tracing.End(span, &err)
	if err := r.store.UpdateJobStatus(ctx, id, status); err != nil {
		return domain.ErrInternal.WithMessage("failed to update status")
	}
	return nil
}

func (r *repositoryService) SetNotes(ctx context.Context, id, notes string) (err error) {
	ctx, span := tracing.Start(ctx, "job.repository.SetNotes")
	defer tracing.End(span, &err)
	if err := r.store.UpdateJobNotes(ctx, id, notes); err != nil {
		return domain.ErrInternal.WithMessage("failed to update notes")
	}
	return nil
}

func (r *repositoryService) MarkApplied(ctx context.Context, id string) error {
	return r.SetStatus(ctx, id, jobcommon.JobStatusApplied)
}
