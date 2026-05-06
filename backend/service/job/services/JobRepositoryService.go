package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/gorm"
)

// jobStore is the GORM-backed implementation of JobReader + JobWriter.
// It holds NO per-user state — joins to user_job_state happen one level
// up in the impl service via the UserJobStateRepository.
type jobStore struct {
	db *gorm.DB
}

// NewJobStore constructs a JobStore for the global jobs table.
func NewJobStore(db *gorm.DB) JobStore {
	return &jobStore{db: db}
}

// encodeCursor packs (added_at, id) into an opaque base64 string. Callers
// treat the cursor as opaque; only this file knows the layout.
func encodeCursor(t time.Time, id string) string {
	raw := fmt.Sprintf("%d|%s", t.UnixNano(), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor unpacks an opaque cursor back into (added_at, id). An empty
// cursor returns the zero values, signalling "first page".
func decodeCursor(cursor string) (time.Time, string, error) {
	if cursor == "" {
		return time.Time{}, "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", errors.ErrBadRequest.WithMessage("invalid cursor")
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", errors.ErrBadRequest.WithMessage("invalid cursor")
	}
	var nano int64
	if _, err := fmt.Sscanf(parts[0], "%d", &nano); err != nil {
		return time.Time{}, "", errors.ErrBadRequest.WithMessage("invalid cursor")
	}
	return time.Unix(0, nano), parts[1], nil
}

// GetJobs returns up to limit+1 rows ordered by (added_at DESC, id DESC).
// The +1 row, if present, becomes the next cursor. The returned slice
// always contains <= limit rows. Filters apply only to the global jobs
// table — per-user state filtering (saved/applied/dismissed) happens in
// the impl service AFTER cache + overlay.
func (s *jobStore) GetJobs(ctx context.Context, filter dto.JobFilter, cursor string, limit int) (_ []jobmodel.Job, _ string, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobs")
	defer tracing.End(span, &err)

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	cursorAt, cursorID, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", err
	}

	query := s.db.WithContext(ctx).Model(&jobmodel.Job{}).Joins("Company")

	// Keyset window: rows strictly older than the cursor (added_at, id).
	if !cursorAt.IsZero() {
		query = query.Where(
			"(jobs.added_at, jobs.id) < (?, ?)",
			cursorAt, cursorID,
		)
	}

	if filter.Search != "" {
		term := "%" + filter.Search + "%"
		query = query.Where(
			"(jobs.title ILIKE ? OR jobs.location ILIKE ? OR companies.name ILIKE ?)",
			term, term, term,
		)
	}

	if len(filter.Companies) > 0 {
		query = query.Where("companies.name IN ?", filter.Companies)
	}

	if filter.Remote != nil {
		query = query.Where("jobs.remote = ?", *filter.Remote)
	}
	if filter.OnSite {
		query = query.Where("jobs.remote = ?", false)
	}

	if len(filter.EmployTypes) > 0 {
		query = query.Where("jobs.employ_type IN ?", filter.EmployTypes)
	}

	if filter.Sponsorship != nil {
		query = query.Where("jobs.sponsorship = ?", *filter.Sponsorship)
	}

	if filter.NoRequirements {
		query = query.Where("jobs.requires_citizen = ? AND jobs.security_clearance = ?", false, false)
	}
	if filter.RequiresCitizen != nil {
		query = query.Where("jobs.requires_citizen = ?", *filter.RequiresCitizen)
	}

	if len(filter.ExperienceRanges) > 0 {
		var clauses []string
		var args []any
		for _, r := range filter.ExperienceRanges {
			if r.Max != nil {
				if r.Min == 0 {
					clauses = append(clauses, "(jobs.experience_years <= ? OR jobs.experience_years IS NULL)")
					args = append(args, *r.Max)
				} else {
					clauses = append(clauses, "(jobs.experience_years > ? AND jobs.experience_years <= ?)")
					args = append(args, r.Min, *r.Max)
				}
			} else {
				clauses = append(clauses, "jobs.experience_years > ?")
				args = append(args, r.Min)
			}
		}
		if len(clauses) > 0 {
			query = query.Where("("+strings.Join(clauses, " OR ")+")", args...)
		}
	}

	if filter.DaysAdded != nil {
		query = query.Where("jobs.added_at >= NOW() - ? * INTERVAL '1 day'", *filter.DaysAdded)
	}

	// Build ORDER BY. The tiebreaker (jobs.added_at DESC, jobs.id DESC) is
	// always appended so cursor-based pagination remains stable across pages
	// when a custom primary sort is in use. The cursor encodes (added_at, id),
	// so the secondary sort on added_at preserves page boundaries.
	var jobs []jobmodel.Job
	if err = query.Order(sortOrderClause(filter.SortBy, filter.SortDir)).Limit(limit + 1).Find(&jobs).Error; err != nil {
		return nil, "", err
	}

	var next string
	if len(jobs) > limit {
		last := jobs[limit-1]
		next = encodeCursor(last.AddedAt, last.ID)
		jobs = jobs[:limit]
	}
	return jobs, next, nil
}

// sortOrderClause returns a safe ORDER BY clause from the caller-supplied sort
// fields. Only whitelisted column names are accepted; unknown values fall back
// to the default (added_at DESC). The tiebreaker is always appended so keyset
// cursors encoded as (added_at, id) continue to bound subsequent pages.
func sortOrderClause(sortBy, sortDir string) string {
	dir := "DESC"
	if sortDir == "asc" {
		dir = "ASC"
	}
	switch sortBy {
	case "title":
		return "jobs.title " + dir + ", jobs.added_at DESC, jobs.id DESC"
	case "company":
		return "companies.name " + dir + ", jobs.added_at DESC, jobs.id DESC"
	case "experience_years":
		return "jobs.experience_years " + dir + " NULLS LAST, jobs.added_at DESC, jobs.id DESC"
	default:
		// added_at or unrecognised — default ordering
		return "jobs.added_at " + dir + ", jobs.id DESC"
	}
}

// GetJobByID returns the global job row plus its company; per-user state
// is overlaid by the impl service.
func (s *jobStore) GetJobByID(ctx context.Context, id string) (_ *jobmodel.Job, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobByID")
	defer tracing.End(span, &err)
	var job jobmodel.Job
	if err = s.db.WithContext(ctx).Joins("Company").Where("jobs.id = ?", id).First(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// GetJobsByIDs returns the global job rows for the given IDs in arbitrary
// order. Used by the impl service to assemble views from cached cursors.
func (s *jobStore) GetJobsByIDs(ctx context.Context, ids []string) (_ []jobmodel.Job, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobsByIDs")
	defer tracing.End(span, &err)
	if len(ids) == 0 {
		return nil, nil
	}
	var jobs []jobmodel.Job
	if err = s.db.WithContext(ctx).Joins("Company").Where("jobs.id IN ?", ids).Find(&jobs).Error; err != nil {
		return nil, err
	}
	return jobs, nil
}

func (s *jobStore) GetJobPostIDs(ctx context.Context, ids []string) (jobPostIDs []string, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobPostIDs")
	defer tracing.End(span, &err)
	if len(ids) == 0 {
		return nil, nil
	}
	err = s.db.WithContext(ctx).Model(&jobmodel.Job{}).
		Where("id IN ?", ids).
		Pluck("job_post_id", &jobPostIDs).Error
	return jobPostIDs, err
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

// CountAll returns the total number of rows in the jobs table. Used to
// derive the per-user "new" count (total − sum(per-user statuses)).
func (s *jobStore) CountAll(ctx context.Context) (_ int64, err error) {
	ctx, span := tracing.Start(ctx, "job.store.CountAll")
	defer tracing.End(span, &err)
	var n int64
	if err = s.db.WithContext(ctx).Model(&jobmodel.Job{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// GetDistinctCompanyNames returns all distinct company names that appear in
// the jobs table, sorted alphabetically.
func (s *jobStore) GetDistinctCompanyNames(ctx context.Context) (_ []string, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetDistinctCompanyNames")
	defer tracing.End(span, &err)
	var names []string
	if err = s.db.WithContext(ctx).
		Table("jobs").
		Joins("JOIN companies ON companies.id = jobs.company_id").
		Distinct("companies.name").
		Order("companies.name").
		Pluck("companies.name", &names).Error; err != nil {
		return nil, err
	}
	return names, nil
}
