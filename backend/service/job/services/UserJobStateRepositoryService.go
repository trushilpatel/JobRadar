package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ UserJobStateRepository = (*userJobStateRepository)(nil)

// userJobStateRepository is the GORM-backed terminal repo for the
// user_job_state table. Every method takes userID explicitly; an empty
// userID is rejected with ErrInternal.WithMessage("user id required") so
// a forgotten ctx scoping never silently broadens a query to all users.
//
// Counter maintenance: every mutation updates user_job_counters atomically
// in the same transaction. Reads short-circuit through the counter table;
// when the counter row is missing, CountsByUser falls back to a GROUP BY
// scan and seeds the counter row so subsequent reads are fast.
type userJobStateRepository struct {
	db  *gorm.DB
	log logger.Logger
}

// NewUserJobStateRepository constructs a UserJobStateRepository.
// An optional logger may be passed as the second argument; when absent a
// no-op logger is used so existing callers (tests, wire_gen.go) are unaffected.
func NewUserJobStateRepository(db *gorm.DB, log ...logger.Logger) UserJobStateRepository {
	var l logger.Logger
	if len(log) > 0 && log[0] != nil {
		l = log[0]
	} else {
		l = logger.NewNoop()
	}
	return &userJobStateRepository{db: db, log: l}
}

// errNoUser is the canonical guard error for empty userID. Same error
// instance for every method so AssertUserScoped tests have a stable check.
func errNoUser() error {
	return apperr.ErrInternal.WithMessage("user id required")
}

// Get returns the row for (userID, jobID), or nil + ErrNotFound if no row
// exists (which means the user has not interacted with this job).
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
		return nil, fmt.Errorf("user_job_state get: %w", err)
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
		return nil, fmt.Errorf("user_job_state get many: %w", err)
	}
	out := make(map[string]*jobmodel.UserJobState, len(rows))
	for i := range rows {
		out[rows[i].JobID] = &rows[i]
	}
	return out, nil
}

// Upsert inserts or updates the row for (userID, jobID) according to the
// requested status. Semantics:
//
//   - state.Status == JobStatusSaved: set saved_at = NOW(); do NOT write to
//     the status column. saved is orthogonal to the workflow status.
//   - state.Status == JobStatusNew: clear the workflow status (status =
//     NULL). Leaves saved_at unchanged.
//   - state.Status is a workflow value (applied / dismissed / interviewing
//     / offer / rejected): set status = X and the matching timestamp.
//     Leaves saved_at unchanged.
//
// Counter maintenance runs in the same transaction. A read of the prior
// row computes the delta; the new row is written; the counter row is
// updated by delta.
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

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		prior, err := loadPriorState(tx, userID, jobID)
		if err != nil {
			return err
		}

		now := time.Now()

		// Decide which columns the upsert writes. saved is orthogonal to
		// workflow status: JobStatusSaved touches saved_at only;
		// JobStatusNew clears workflow status only; workflow values touch
		// status + their timestamp.
		row := jobmodel.UserJobState{
			UserID:          userID,
			JobID:           jobID,
			Notes:           state.Notes,
			DismissalReason: state.DismissalReason,
			DismissalNote:   state.DismissalNote,
		}

		updateCols := []string{"updated_at"}
		switch state.Status {
		case jobcommon.JobStatusSaved:
			// Orthogonal save — set saved_at, preserve any workflow status
			// already on the row.
			row.SavedAt = &now
			if prior != nil {
				row.Status = prior.Status
			}
			updateCols = append(updateCols, "saved_at")
		case jobcommon.JobStatusNew:
			// Clear workflow status; leave saved_at alone.
			row.Status = nil
			if prior != nil {
				row.SavedAt = prior.SavedAt
			}
			updateCols = append(updateCols, "status")
		case jobmodel.StatusApplied:
			s := jobmodel.StatusApplied
			row.Status = &s
			row.AppliedAt = &now
			if prior != nil {
				row.SavedAt = prior.SavedAt
			}
			updateCols = append(updateCols, "status", "applied_at")
		case jobmodel.StatusDismissed:
			s := jobmodel.StatusDismissed
			row.Status = &s
			row.DismissedAt = &now
			if prior != nil {
				row.SavedAt = prior.SavedAt
			}
			updateCols = append(updateCols, "status", "dismissed_at", "dismissal_reason", "dismissal_note")
		case jobmodel.StatusInterviewing, jobmodel.StatusOffer, jobmodel.StatusRejected:
			s := state.Status
			row.Status = &s
			if prior != nil {
				row.SavedAt = prior.SavedAt
			}
			updateCols = append(updateCols, "status")
		default:
			return apperr.ErrBadRequest.WithMessage("invalid status")
		}
		if state.Notes != nil {
			updateCols = append(updateCols, "notes")
		}

		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "job_id"}},
			DoUpdates: clause.AssignmentColumns(updateCols),
		}).Create(&row).Error; err != nil {
			return fmt.Errorf("user_job_state upsert: %w", err)
		}

		delta := counterDelta(priorSnapshot(prior), newSnapshot(prior, state.Status))
		if !delta.zero() {
			if err := applyCounterDelta(tx, userID, delta); err != nil {
				return fmt.Errorf("user_job_counters update: %w", err)
			}
		}
		return nil
	})
}

// SetSaved is the orthogonal save/unsave path. saved=true sets saved_at to
// NOW(); saved=false clears saved_at. The workflow status column is left
// untouched. Idempotent — calling SetSaved(true) twice does not double-
// increment the counter.
func (r *userJobStateRepository) SetSaved(ctx context.Context, userID, jobID string, saved bool) (err error) {
	ctx, span := tracing.Start(ctx, "job.userstate.SetSaved")
	defer tracing.End(span, &err)

	if userID == "" {
		return errNoUser()
	}
	if jobID == "" {
		return apperr.ErrBadRequest.WithMessage("job id required")
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		prior, err := loadPriorState(tx, userID, jobID)
		if err != nil {
			return err
		}

		wasSaved := prior != nil && prior.SavedAt != nil
		// Idempotent — no-op when the requested state already matches.
		if saved && wasSaved {
			return nil
		}
		if !saved && prior == nil {
			return nil
		}

		if saved {
			now := time.Now()
			row := jobmodel.UserJobState{
				UserID:  userID,
				JobID:   jobID,
				SavedAt: &now,
			}
			if prior != nil {
				row.Status = prior.Status
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "user_id"}, {Name: "job_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"saved_at", "updated_at"}),
			}).Create(&row).Error; err != nil {
				return fmt.Errorf("user_job_state set saved: %w", err)
			}
			if err := applyCounterDelta(tx, userID, counterChange{savedDelta: 1}); err != nil {
				return fmt.Errorf("user_job_counters increment saved: %w", err)
			}
			return nil
		}

		// saved=false — clear saved_at. If the row has no other meaningful
		// field, delete it entirely so the non_empty CHECK constraint is
		// not violated and the row count stays sparse.
		if rowIsMeaningfulWithoutSaved(prior) {
			if err := tx.Model(&jobmodel.UserJobState{}).
				Where("user_id = ? AND job_id = ?", userID, jobID).
				Updates(map[string]any{"saved_at": nil, "updated_at": time.Now()}).Error; err != nil {
				return fmt.Errorf("user_job_state clear saved: %w", err)
			}
		} else {
			if err := tx.Where("user_id = ? AND job_id = ?", userID, jobID).
				Delete(&jobmodel.UserJobState{}).Error; err != nil {
				return fmt.Errorf("user_job_state delete empty: %w", err)
			}
		}
		if err := applyCounterDelta(tx, userID, counterChange{savedDelta: -1}); err != nil {
			return fmt.Errorf("user_job_counters decrement saved: %w", err)
		}
		return nil
	})
}

// UpsertBulkDismiss writes (or updates) one row per jobID with
// status='dismissed', the same reason+note, and dismissed_at=NOW(). The
// counter delta is computed from the prior state of each row so a job that
// was already dismissed does not double-count.
func (r *userJobStateRepository) UpsertBulkDismiss(ctx context.Context, userID string, jobIDs []string, reason, note string) (err error) {
	ctx, span := tracing.Start(ctx, "job.userstate.UpsertBulkDismiss")
	defer tracing.End(span, &err)

	if userID == "" {
		return errNoUser()
	}
	if len(jobIDs) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Read the prior state for every target row so the counter delta
		// is exact (a job already-dismissed contributes 0, not +1).
		var priorRows []jobmodel.UserJobState
		if err := tx.Where("user_id = ? AND job_id IN ?", userID, jobIDs).
			Find(&priorRows).Error; err != nil {
			return fmt.Errorf("user_job_state bulk prior read: %w", err)
		}
		priorByJob := make(map[string]*jobmodel.UserJobState, len(priorRows))
		for i := range priorRows {
			priorByJob[priorRows[i].JobID] = &priorRows[i]
		}

		now := time.Now()
		dismissed := jobmodel.StatusDismissed
		var notePtr *string
		if note != "" {
			notePtr = &note
		}
		reasonPtr := &reason

		rows := make([]jobmodel.UserJobState, 0, len(jobIDs))
		var total counterChange
		for _, id := range jobIDs {
			row := jobmodel.UserJobState{
				UserID:          userID,
				JobID:           id,
				Status:          &dismissed,
				DismissedAt:     &now,
				DismissalReason: reasonPtr,
				DismissalNote:   notePtr,
			}
			if prior, ok := priorByJob[id]; ok && prior != nil {
				row.SavedAt = prior.SavedAt
			}
			rows = append(rows, row)
			total = total.add(counterDelta(
				priorSnapshot(priorByJob[id]),
				newSnapshot(priorByJob[id], jobmodel.StatusDismissed),
			))
		}

		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "job_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"status", "dismissed_at", "dismissal_reason", "dismissal_note", "updated_at",
			}),
		}).CreateInBatches(rows, 500).Error; err != nil {
			return fmt.Errorf("user_job_state bulk dismiss: %w", err)
		}

		if !total.zero() {
			if err := applyCounterDelta(tx, userID, total); err != nil {
				return fmt.Errorf("user_job_counters bulk dismiss: %w", err)
			}
		}
		return nil
	})
}

// Delete removes the row for (userID, jobID). The counter delta is the
// negative of the prior row's contribution so saved/applied/etc. counts
// stay in sync with the table.
func (r *userJobStateRepository) Delete(ctx context.Context, userID, jobID string) (err error) {
	ctx, span := tracing.Start(ctx, "job.userstate.Delete")
	defer tracing.End(span, &err)

	if userID == "" {
		return errNoUser()
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		prior, err := loadPriorState(tx, userID, jobID)
		if err != nil {
			return err
		}
		if prior == nil {
			return nil // nothing to delete, no counter change
		}
		if err := tx.Where("user_id = ? AND job_id = ?", userID, jobID).
			Delete(&jobmodel.UserJobState{}).Error; err != nil {
			return fmt.Errorf("user_job_state delete: %w", err)
		}
		delta := counterChange{}.sub(priorSnapshot(prior))
		if !delta.zero() {
			if err := applyCounterDelta(tx, userID, delta); err != nil {
				return fmt.Errorf("user_job_counters delete: %w", err)
			}
		}
		return nil
	})
}

// CountsByUser returns the cached per-user counts from user_job_counters.
// Falls back to a GROUP BY recompute (and seeds the counter row) if no row
// exists for the user yet.
func (r *userJobStateRepository) CountsByUser(ctx context.Context, userID string) (_ dto.Counts, err error) {
	ctx, span := tracing.Start(ctx, "job.userstate.CountsByUser")
	defer tracing.End(span, &err)

	if userID == "" {
		return dto.Counts{}, errNoUser()
	}

	var counter jobmodel.UserJobCounters
	err = r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		First(&counter).Error
	if err == nil {
		return dto.Counts{
			Saved:        counter.SavedCount,
			Applied:      counter.AppliedCount,
			Interviewing: counter.InterviewingCount,
			Offer:        counter.OfferCount,
			Rejected:     counter.RejectedCount,
			Dismissed:    counter.DismissedCount,
		}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return dto.Counts{}, fmt.Errorf("user_job_counters read: %w", err)
	}

	// Fallback — recompute from user_job_state and seed the counter row so
	// future reads are O(1).
	counts, err := recomputeCounts(ctx, r.db, userID)
	if err != nil {
		return dto.Counts{}, err
	}
	// Seeding failure is non-fatal — the counts are still correct for this
	// call; the next call will retry the seed.
	if err := seedCounter(ctx, r.db, userID, counts); err != nil {
		r.log.Warn("user job state counter seed failed", logger.Err(err), logger.Str(logger.FieldUserID, userID))
	}
	return counts, nil
}

// GetJobsByStatus returns up to limit jobs for userID with the requested status,
// honouring all additional filter criteria in filter (search, companies, date,
// experience range, remote/on-site). Drives from user_job_state so every page
// is a full limit-sized result set. Cursor format matches GetJobs.
//
// filter.Statuses (when non-empty) takes precedence and joins on
// ujs.status IN (...) for a combined multi-stage view (e.g. the Tracking tab).
// Otherwise: Status "saved" joins on saved_at IS NOT NULL (orthogonal flag);
// other workflow values join on ujs.status = ? AND ujs.status IS NOT NULL.
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
		Joins("Company")

	switch {
	case len(filter.Statuses) > 0:
		query = query.Joins(
			"INNER JOIN user_job_state ujs ON ujs.job_id = jobs.id AND ujs.user_id = ? AND ujs.status IN ? AND ujs.status IS NOT NULL",
			userID, filter.Statuses,
		)
	case filter.Status == jobcommon.JobStatusSaved:
		query = query.Joins(
			"INNER JOIN user_job_state ujs ON ujs.job_id = jobs.id AND ujs.user_id = ? AND ujs.saved_at IS NOT NULL",
			userID,
		)
	default:
		query = query.Joins(
			"INNER JOIN user_job_state ujs ON ujs.job_id = jobs.id AND ujs.user_id = ? AND ujs.status = ? AND ujs.status IS NOT NULL",
			userID, filter.Status,
		)
	}

	query = applyCursorPredicate(query, p)

	// Apply the same filter predicates as GetJobs so search/companies/date/etc.
	// work consistently across all status tabs, not just the "new" tab.
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
		return nil, "", fmt.Errorf("user_job_state get jobs by status: %w", err)
	}

	var next string
	if len(jobs) > limit {
		next = buildCursor(filter.SortBy, filter.SortDir, jobs[limit-1])
		jobs = jobs[:limit]
	}
	return jobs, next, nil
}

// loadPriorState reads the existing (userID, jobID) row for use in delta
// computations. Returns nil + nil error when no row exists.
func loadPriorState(tx *gorm.DB, userID, jobID string) (*jobmodel.UserJobState, error) {
	var row jobmodel.UserJobState
	err := tx.Where("user_id = ? AND job_id = ?", userID, jobID).First(&row).Error
	if err == nil {
		return &row, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("load prior state: %w", err)
}

// rowIsMeaningfulWithoutSaved decides whether clearing saved_at would
// leave a meaningful row. The non_empty_check CHECK constraint requires at
// least one populated field; drop the row when nothing else remains.
func rowIsMeaningfulWithoutSaved(s *jobmodel.UserJobState) bool {
	if s == nil {
		return false
	}
	if s.Status != nil {
		return true
	}
	if s.AppliedAt != nil || s.DismissedAt != nil {
		return true
	}
	if s.Notes != nil || s.DismissalReason != nil || s.DismissalNote != nil {
		return true
	}
	return false
}
