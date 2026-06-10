package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// cursorPayload is the sort-aware keyset cursor state serialised between
// pages. All fields are exported for JSON round-tripping; callers treat the
// base64-encoded string as opaque.
type cursorPayload struct {
	SortBy  string `json:"s"` // "added_at" | "title" | "company" | "experience_years"
	SortDir string `json:"d"` // "asc" | "desc"
	PrimVal string `json:"v"` // primary sort column value as string; "" means NULL / added_at sort
	AddedAt int64  `json:"a"` // added_at.UnixNano() for tiebreaking
	ID      string `json:"i"` // job ID for final tiebreaking
}

// encodeCursor packs the sort field, direction, primary sort value and the
// (added_at, id) tiebreakers into an opaque base64-encoded JSON cursor.
func encodeCursor(sortBy, sortDir, primVal string, addedAt time.Time, id string) string {
	p := cursorPayload{
		SortBy:  sortBy,
		SortDir: sortDir,
		PrimVal: primVal,
		AddedAt: addedAt.UnixNano(),
		ID:      id,
	}
	b, _ := json.Marshal(p)
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeCursor unpacks an opaque cursor. An empty cursor returns nil (first
// page). Unrecognised or corrupt formats also return nil so stale client-side
// cursors gracefully restart from page 1 instead of surfacing a 400 error.
func decodeCursor(cursor string) (*cursorPayload, error) {
	if cursor == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, nil // undecodable → first page
	}
	var p cursorPayload
	if err = json.Unmarshal(raw, &p); err != nil || p.ID == "" {
		return nil, nil // old format or corrupted → first page
	}
	return &p, nil
}

// buildCursor constructs the correct opaque cursor for the last row on the
// current page given the active sort column and direction.
func buildCursor(sortBy, sortDir string, job jobmodel.Job) string {
	var primVal string
	switch sortBy {
	case "title":
		primVal = job.Title
	case "company":
		primVal = job.Company.Name
	case "experience_years":
		if job.ExperienceYears != nil {
			primVal = strconv.Itoa(*job.ExperienceYears)
		} // nil ExperienceYears → primVal stays "" (signals NULL)
		// "added_at" and unrecognised: primVal stays ""; tiebreakers carry the position
	}
	return encodeCursor(sortBy, sortDir, primVal, job.AddedAt, job.ID)
}

// applyCursorPredicate adds the keyset WHERE predicate that bounds the next
// page based on the active sort column. p == nil means first page (no WHERE
// applied). The predicate mirrors the ORDER BY produced by sortOrderClause.
func applyCursorPredicate(query *gorm.DB, p *cursorPayload) *gorm.DB {
	if p == nil {
		return query
	}
	addedAt := time.Unix(0, p.AddedAt)

	switch p.SortBy {
	case "title":
		// ORDER: jobs.title {dir}, jobs.added_at DESC, jobs.id DESC
		if p.SortDir == "asc" {
			return query.Where(
				"jobs.title > ? OR (jobs.title = ? AND (jobs.added_at, jobs.id) < (?, ?))",
				p.PrimVal, p.PrimVal, addedAt, p.ID,
			)
		}
		return query.Where(
			"jobs.title < ? OR (jobs.title = ? AND (jobs.added_at, jobs.id) < (?, ?))",
			p.PrimVal, p.PrimVal, addedAt, p.ID,
		)

	case "company":
		// ORDER: "Company".name {dir}, jobs.added_at DESC, jobs.id DESC.
		// GORM's Joins("Company") aliases the companies table as "Company";
		// references to the unaliased name fail with "missing FROM-clause
		// entry for table \"companies\"" in Postgres.
		if p.SortDir == "asc" {
			return query.Where(
				`"Company".name > ? OR ("Company".name = ? AND (jobs.added_at, jobs.id) < (?, ?))`,
				p.PrimVal, p.PrimVal, addedAt, p.ID,
			)
		}
		return query.Where(
			`"Company".name < ? OR ("Company".name = ? AND (jobs.added_at, jobs.id) < (?, ?))`,
			p.PrimVal, p.PrimVal, addedAt, p.ID,
		)

	case "experience_years":
		return applyExperienceCursorPredicate(query, p, addedAt)

	default: // "added_at" and unrecognised
		// ORDER: jobs.added_at {dir}, jobs.id DESC
		if p.SortDir == "asc" {
			// Next-page rows have a newer added_at, or same added_at with a smaller id.
			return query.Where(
				"jobs.added_at > ? OR (jobs.added_at = ? AND jobs.id < ?)",
				addedAt, addedAt, p.ID,
			)
		}
		// Default (desc): rows strictly older than the cursor.
		return query.Where("(jobs.added_at, jobs.id) < (?, ?)", addedAt, p.ID)
	}
}

// applyExperienceCursorPredicate handles the nullable experience_years column
// with NULLS LAST semantics (matches sortOrderClause).
func applyExperienceCursorPredicate(query *gorm.DB, p *cursorPayload, addedAt time.Time) *gorm.DB {
	const tiebreak = "(jobs.added_at, jobs.id) < (?, ?)"

	if p.PrimVal == "" {
		// Cursor sits on a NULL row: next page is more NULL rows, tiebroken.
		return query.Where("jobs.experience_years IS NULL AND "+tiebreak, addedAt, p.ID)
	}

	expYears, _ := strconv.Atoi(p.PrimVal)

	if p.SortDir == "asc" {
		// ASC NULLS LAST: 0, 1, 2, ..., NULL
		return query.Where(
			"(jobs.experience_years > ?) OR (jobs.experience_years = ? AND "+tiebreak+") OR jobs.experience_years IS NULL",
			expYears, expYears, addedAt, p.ID,
		)
	}
	// DESC NULLS LAST: MAX, ..., 1, 0, NULL
	return query.Where(
		"(jobs.experience_years < ?) OR (jobs.experience_years = ? AND "+tiebreak+") OR jobs.experience_years IS NULL",
		expYears, expYears, addedAt, p.ID,
	)
}

// GetJobs returns up to limit+1 rows ordered by sortOrderClause.
// The +1 probe row drives next-cursor generation. Filters apply only to the
// global jobs table; per-user state filtering happens in the impl service.
func (s *jobStore) GetJobs(ctx context.Context, filter dto.JobFilter, cursor string, limit int) (_ []jobmodel.Job, _ string, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobs")
	defer tracing.End(span, &err)

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	p, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", err
	}

	query := s.db.WithContext(ctx).Model(&jobmodel.Job{}).Joins("Company")
	query = applyCursorPredicate(query, p)

	if filter.Search != "" {
		term := "%" + filter.Search + "%"
		// "Company".name — GORM's Joins("Company") aliases the joined table
		// as "Company"; the original "companies" name is out of scope.
		query = query.Where(
			`(jobs.title ILIKE ? OR jobs.location ILIKE ? OR "Company".name ILIKE ?)`,
			term, term, term,
		)
	}

	if len(filter.Companies) > 0 {
		query = query.Where(`"Company".name IN ?`, filter.Companies)
	}

	if filter.Remote != nil {
		query = query.Where("jobs.remote = ?", *filter.Remote)
	}
	if filter.OnSite {
		query = query.Where("jobs.remote = ?", false)
	}
	if filter.Hybrid {
		query = query.Where("LOWER(jobs.employ_type) LIKE ?", "%hybrid%")
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

	var jobs []jobmodel.Job
	if err = query.Order(sortOrderClause(filter.SortBy, filter.SortDir)).Limit(limit + 1).Find(&jobs).Error; err != nil {
		return nil, "", err
	}

	var next string
	if len(jobs) > limit {
		next = buildCursor(filter.SortBy, filter.SortDir, jobs[limit-1])
		jobs = jobs[:limit]
	}
	return jobs, next, nil
}

// sortOrderClause returns a safe ORDER BY clause from the caller-supplied sort
// fields. Only whitelisted column names are accepted; unknown values fall back
// to the default (added_at DESC). The tiebreaker (added_at DESC, id DESC) is
// always appended so keyset cursors encoded as (sortVal, added_at, id) continue
// to bound subsequent pages correctly.
func sortOrderClause(sortBy, sortDir string) string {
	dir := "DESC"
	if sortDir == "asc" {
		dir = "ASC"
	}
	switch sortBy {
	case "title":
		return "jobs.title " + dir + ", jobs.added_at DESC, jobs.id DESC"
	case "company":
		// "Company".name — alias produced by Joins("Company"). Using the
		// unaliased "companies.name" errors with "missing FROM-clause entry".
		return `"Company".name ` + dir + ", jobs.added_at DESC, jobs.id DESC"
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

// InsertJob is idempotent on job_post_id. A replay of the same job_post_id
// (crawler→filter→enricher re-discovery) is a no-op via ON CONFLICT DO NOTHING,
// and returns ErrJobAlreadyExists so callers can skip dependent writes (snapshot
// + tech-tags) that would key off a UUID that was never persisted.
func (s *jobStore) InsertJob(ctx context.Context, job *jobmodel.Job) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertJob")
	defer tracing.End(span, &err)
	res := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "job_post_id"}}, DoNothing: true}).
		Create(job)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domainerrs.ErrJobAlreadyExists.WithMessage(
			"InsertJob: job_post_id already exists: " + job.JobPostID)
	}
	return nil
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

// CountAll returns the total number of rows in the jobs table.
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
