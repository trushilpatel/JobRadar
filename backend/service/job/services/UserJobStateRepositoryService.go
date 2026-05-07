package services

import (
	"context"
	"errors"
	"strings"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// userJobStateRepository is the GORM-backed terminal repo for the
// user_job_state table. Every method takes userID explicitly; an empty
// userID is rejected with ErrInternal.WithMessage("user id required") so
// a forgotten ctx scoping never silently broadens a query to all users.
type userJobStateRepository struct {
	db *gorm.DB
}

// NewUserJobStateRepository constructs a UserJobStateRepository.
func NewUserJobStateRepository(db *gorm.DB) UserJobStateRepository {
	return &userJobStateRepository{db: db}
}

// errNoUser is the canonical guard error for empty userID. Same error
// instance for every method so AssertUserScoped tests have a stable check.
func errNoUser() error {
	return apperr.ErrInternal.WithMessage("user id required")
}

// Get returns the row for (userID, jobID), or nil + ErrNotFound if no row
// exists (which means status is implicitly "new").
func (r *userJobStateRepository) Get(ctx context.Context, userID, jobID string) (_ *jobmodel.UserJobState, err error) {
	ctx, span := tracing.Start(ctx, "job.userstate.Get")
	defer tracing.End(span, &err)

	if userID == "" {
		return nil, errNoUser()
	}
	var row jobmodel.UserJobState
	err = r.db.WithContext(ctx).
		Where("user_id = ? AND job_id = ?", userID, jobID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrNotFound.WithMessage("user_job_state not found")
		}
		return nil, err
	}
	return &row, nil
}

// GetMany returns user_job_state rows for many job IDs in a single query.
// The map is keyed by job_id; jobs the user has not interacted with are
// simply absent from the map (caller treats absence as "new").
func (r *userJobStateRepository) GetMany(ctx context.Context, userID string, jobIDs []string) (_ map[string]*jobmodel.UserJobState, err error) {
	ctx, span := tracing.Start(ctx, "job.userstate.GetMany")
	defer tracing.End(span, &err)

	if userID == "" {
		return nil, errNoUser()
	}
	if len(jobIDs) == 0 {
		return map[string]*jobmodel.UserJobState{}, nil
	}
	var rows []jobmodel.UserJobState
	if err = r.db.WithContext(ctx).
		Where("user_id = ? AND job_id IN ?", userID, jobIDs).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]*jobmodel.UserJobState, len(rows))
	for i := range rows {
		out[rows[i].JobID] = &rows[i]
	}
	return out, nil
}

// Upsert inserts or updates the row for (userID, jobID) using ON CONFLICT.
// Status drives which timestamp column is set (saved_at / applied_at /
// dismissed_at) — only the active-status timestamp is updated to NOW(),
// the others retain their previous value.
func (r *userJobStateRepository) Upsert(ctx context.Context, userID, jobID string, state dto.UpsertState) (err error) {
	ctx, span := tracing.Start(ctx, "job.userstate.Upsert")
	defer tracing.End(span, &err)

	if userID == "" {
		return errNoUser()
	}
	if jobID == "" {
		return apperr.ErrBadRequest.WithMessage("job id required")
	}
	if state.Status == "" {
		return apperr.ErrBadRequest.WithMessage("status required")
	}

	now := time.Now()
	row := jobmodel.UserJobState{
		UserID:          userID,
		JobID:           jobID,
		Status:          state.Status,
		Notes:           state.Notes,
		DismissalReason: state.DismissalReason,
		DismissalNote:   state.DismissalNote,
	}
	switch state.Status {
	case jobmodel.StatusSaved:
		row.SavedAt = &now
	case jobmodel.StatusApplied:
		row.AppliedAt = &now
	case jobmodel.StatusDismissed:
		row.DismissedAt = &now
	}

	// Build the column-set list explicitly so we never overwrite an
	// already-populated timestamp column with NULL on a status flip.
	updateCols := []string{"status", "updated_at"}
	switch state.Status {
	case jobmodel.StatusSaved:
		updateCols = append(updateCols, "saved_at")
	case jobmodel.StatusApplied:
		updateCols = append(updateCols, "applied_at")
	case jobmodel.StatusDismissed:
		updateCols = append(updateCols, "dismissed_at", "dismissal_reason", "dismissal_note")
	}
	if state.Notes != nil {
		updateCols = append(updateCols, "notes")
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "job_id"}},
		DoUpdates: clause.AssignmentColumns(updateCols),
	}).Create(&row).Error
}

// UpsertBulkDismiss writes (or updates) one row per jobID with
// status='dismissed', the same reason+note, and dismissed_at=NOW(). All
// rows are written in a single CreateInBatches call wrapped in ON CONFLICT.
func (r *userJobStateRepository) UpsertBulkDismiss(ctx context.Context, userID string, jobIDs []string, reason, note string) (err error) {
	ctx, span := tracing.Start(ctx, "job.userstate.UpsertBulkDismiss")
	defer tracing.End(span, &err)

	if userID == "" {
		return errNoUser()
	}
	if len(jobIDs) == 0 {
		return nil
	}
	now := time.Now()
	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	reasonPtr := &reason

	rows := make([]jobmodel.UserJobState, len(jobIDs))
	for i, id := range jobIDs {
		rows[i] = jobmodel.UserJobState{
			UserID:          userID,
			JobID:           id,
			Status:          jobmodel.StatusDismissed,
			DismissedAt:     &now,
			DismissalReason: reasonPtr,
			DismissalNote:   notePtr,
		}
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "job_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "dismissed_at", "dismissal_reason", "dismissal_note", "updated_at",
		}),
	}).CreateInBatches(rows, 500).Error
}

// Delete removes the row for (userID, jobID). Used to clear a job's per-
// user state — equivalent to setting status back to "new".
func (r *userJobStateRepository) Delete(ctx context.Context, userID, jobID string) (err error) {
	ctx, span := tracing.Start(ctx, "job.userstate.Delete")
	defer tracing.End(span, &err)

	if userID == "" {
		return errNoUser()
	}
	return r.db.WithContext(ctx).
		Where("user_id = ? AND job_id = ?", userID, jobID).
		Delete(&jobmodel.UserJobState{}).Error
}

// CountsByUser returns the number of rows in each saved/applied/dismissed
// bucket for the given user. "New" is computed by the caller (it is the
// absence of a row, not a stored value).
func (r *userJobStateRepository) CountsByUser(ctx context.Context, userID string) (_ dto.Counts, err error) {
	ctx, span := tracing.Start(ctx, "job.userstate.CountsByUser")
	defer tracing.End(span, &err)

	if userID == "" {
		return dto.Counts{}, errNoUser()
	}
	var rows []dto.CountsRow
	err = r.db.WithContext(ctx).
		Raw(`SELECT status, COUNT(*) AS count
		     FROM user_job_state
		     WHERE user_id = ?
		     GROUP BY status`, userID).
		Scan(&rows).Error
	if err != nil {
		return dto.Counts{}, err
	}
	out := dto.Counts{}
	for _, r := range rows {
		switch r.Status {
		case jobmodel.StatusSaved:
			out.Saved = r.Count
		case jobmodel.StatusApplied:
			out.Applied = r.Count
		case jobmodel.StatusDismissed:
			out.Dismissed = r.Count
		case jobmodel.StatusInterviewing:
			out.Interviewing = r.Count
		case jobmodel.StatusOffer:
			out.Offer = r.Count
		case jobmodel.StatusRejected:
			out.Rejected = r.Count
		}
	}
	return out, nil
}

// GetJobsByStatus returns up to limit jobs for userID with the requested status,
// honouring all additional filter criteria in filter (search, companies, date,
// experience range, remote/on-site). Drives from user_job_state so every page
// is a full limit-sized result set. Cursor format matches GetJobs.
func (r *userJobStateRepository) GetJobsByStatus(ctx context.Context, userID string, filter dto.JobFilter, cursor string, limit int) (_ []jobmodel.Job, _ string, err error) {
	ctx, span := tracing.Start(ctx, "job.userstate.GetJobsByStatus")
	defer tracing.End(span, &err)

	if userID == "" {
		return nil, "", errNoUser()
	}

	p, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", err
	}

	query := r.db.WithContext(ctx).
		Model(&jobmodel.Job{}).
		Joins("Company").
		Joins("INNER JOIN user_job_state ujs ON ujs.job_id = jobs.id AND ujs.user_id = ? AND ujs.status = ?", userID, filter.Status)

	query = applyCursorPredicate(query, p)

	// Apply the same filter predicates as GetJobs so search/companies/date/etc.
	// work consistently across all status tabs, not just the "new" tab.
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
		for _, rng := range filter.ExperienceRanges {
			if rng.Max != nil {
				if rng.Min == 0 {
					clauses = append(clauses, "(jobs.experience_years <= ? OR jobs.experience_years IS NULL)")
					args = append(args, *rng.Max)
				} else {
					clauses = append(clauses, "(jobs.experience_years > ? AND jobs.experience_years <= ?)")
					args = append(args, rng.Min, *rng.Max)
				}
			} else {
				clauses = append(clauses, "jobs.experience_years > ?")
				args = append(args, rng.Min)
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
