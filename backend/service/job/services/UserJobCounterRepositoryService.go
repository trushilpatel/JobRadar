package services

// Counter machinery
//
// counterSnapshot represents the buckets a single row contributes to. The
// snapshot is taken before and after a write; the delta drives the counter
// table update. Saved is orthogonal to the workflow status, so a row can
// contribute to multiple buckets simultaneously.

import (
	"context"
	"fmt"
	"time"

	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type counterSnapshot struct {
	saved        bool
	applied      bool
	interviewing bool
	offer        bool
	rejected     bool
	dismissed    bool
}

// priorSnapshot derives the bucket flags from an existing UserJobState row.
// nil prior contributes nothing.
func priorSnapshot(prior *jobmodel.UserJobState) counterSnapshot {
	if prior == nil {
		return counterSnapshot{}
	}
	s := counterSnapshot{saved: prior.SavedAt != nil}
	switch prior.StatusOrEmpty() {
	case jobmodel.StatusApplied:
		s.applied = true
	case jobmodel.StatusInterviewing:
		s.interviewing = true
	case jobmodel.StatusOffer:
		s.offer = true
	case jobmodel.StatusRejected:
		s.rejected = true
	case jobmodel.StatusDismissed:
		s.dismissed = true
	}
	return s
}

// newSnapshot derives the bucket flags for the post-write row given the
// prior row (for saved_at and workflow-status preservation) and the
// requested status.
func newSnapshot(prior *jobmodel.UserJobState, status string) counterSnapshot {
	s := counterSnapshot{}
	// Saved is preserved across status-only writes; freshly set on Save.
	if status == jobcommon.JobStatusSaved {
		s.saved = true
	} else if prior != nil && prior.SavedAt != nil {
		s.saved = true
	}
	switch status {
	case jobcommon.JobStatusSaved:
		// Workflow status is unchanged — inherit prior workflow bucket.
		if prior != nil {
			switch prior.StatusOrEmpty() {
			case jobmodel.StatusApplied:
				s.applied = true
			case jobmodel.StatusInterviewing:
				s.interviewing = true
			case jobmodel.StatusOffer:
				s.offer = true
			case jobmodel.StatusRejected:
				s.rejected = true
			case jobmodel.StatusDismissed:
				s.dismissed = true
			}
		}
	case jobcommon.JobStatusNew:
		// Clears workflow status — workflow bucket is empty.
	case jobmodel.StatusApplied:
		s.applied = true
	case jobmodel.StatusInterviewing:
		s.interviewing = true
	case jobmodel.StatusOffer:
		s.offer = true
	case jobmodel.StatusRejected:
		s.rejected = true
	case jobmodel.StatusDismissed:
		s.dismissed = true
	}
	return s
}

// counterChange holds the per-bucket delta to apply to user_job_counters.
type counterChange struct {
	savedDelta        int
	appliedDelta      int
	interviewingDelta int
	offerDelta        int
	rejectedDelta     int
	dismissedDelta    int
}

func (c counterChange) zero() bool {
	return c.savedDelta == 0 && c.appliedDelta == 0 && c.interviewingDelta == 0 &&
		c.offerDelta == 0 && c.rejectedDelta == 0 && c.dismissedDelta == 0
}

func (c counterChange) add(o counterChange) counterChange {
	c.savedDelta += o.savedDelta
	c.appliedDelta += o.appliedDelta
	c.interviewingDelta += o.interviewingDelta
	c.offerDelta += o.offerDelta
	c.rejectedDelta += o.rejectedDelta
	c.dismissedDelta += o.dismissedDelta
	return c
}

// sub subtracts a snapshot's contributions from the change. Used by Delete
// to express "row contributed +1 to bucket X; removing it deltas -1".
func (c counterChange) sub(s counterSnapshot) counterChange {
	if s.saved {
		c.savedDelta--
	}
	if s.applied {
		c.appliedDelta--
	}
	if s.interviewing {
		c.interviewingDelta--
	}
	if s.offer {
		c.offerDelta--
	}
	if s.rejected {
		c.rejectedDelta--
	}
	if s.dismissed {
		c.dismissedDelta--
	}
	return c
}

// counterDelta computes the change required to go from `prior` to `next`.
func counterDelta(prior, next counterSnapshot) counterChange {
	c := counterChange{}
	c.savedDelta += boolDelta(next.saved, prior.saved)
	c.appliedDelta += boolDelta(next.applied, prior.applied)
	c.interviewingDelta += boolDelta(next.interviewing, prior.interviewing)
	c.offerDelta += boolDelta(next.offer, prior.offer)
	c.rejectedDelta += boolDelta(next.rejected, prior.rejected)
	c.dismissedDelta += boolDelta(next.dismissed, prior.dismissed)
	return c
}

func boolDelta(next, prior bool) int {
	switch {
	case next && !prior:
		return 1
	case !next && prior:
		return -1
	default:
		return 0
	}
}

// applyCounterDelta applies a counter change to user_job_counters,
// inserting a zero row first if none exists. All deltas are clamped at 0
// to respect the CHECK (>= 0) constraint via GREATEST(col + delta, 0).
func applyCounterDelta(tx *gorm.DB, userID string, delta counterChange) error {
	// Ensure a row exists. ON CONFLICT DO NOTHING — first writer wins.
	zero := jobmodel.UserJobCounters{UserID: userID}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoNothing: true,
	}).Create(&zero).Error; err != nil {
		return fmt.Errorf("ensure counter row: %w", err)
	}

	// GREATEST(col + delta, 0) is the Postgres scalar two-arg max — clamps
	// each counter at zero so a transient over-decrement can never violate
	// the CHECK (>= 0) constraint. SQLite has no GREATEST builtin; the
	// SQLite-backed test helper registers an equivalent scalar via a
	// ConnectHook so this single SQL works in both engines.
	res := tx.Exec(`
		UPDATE user_job_counters
		   SET saved_count        = GREATEST(saved_count + ?, 0),
		       applied_count      = GREATEST(applied_count + ?, 0),
		       interviewing_count = GREATEST(interviewing_count + ?, 0),
		       offer_count        = GREATEST(offer_count + ?, 0),
		       rejected_count     = GREATEST(rejected_count + ?, 0),
		       dismissed_count    = GREATEST(dismissed_count + ?, 0),
		       updated_at         = ?
		 WHERE user_id = ?`,
		delta.savedDelta, delta.appliedDelta, delta.interviewingDelta,
		delta.offerDelta, delta.rejectedDelta, delta.dismissedDelta,
		time.Now(), userID)
	if res.Error != nil {
		return fmt.Errorf("apply counter delta: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("apply counter delta: counter row missing for user %s", userID)
	}
	return nil
}

// recomputeCounts scans user_job_state and returns the per-bucket totals.
// Used by CountsByUser as a fallback when no counter row exists yet.
func recomputeCounts(ctx context.Context, db *gorm.DB, userID string) (dto.Counts, error) {
	var saved int64
	if err := db.WithContext(ctx).
		Model(&jobmodel.UserJobState{}).
		Where("user_id = ? AND saved_at IS NOT NULL", userID).
		Count(&saved).Error; err != nil {
		return dto.Counts{}, fmt.Errorf("counts recompute saved: %w", err)
	}

	var rows []dto.CountsRow
	if err := db.WithContext(ctx).
		Raw(`SELECT status, COUNT(*) AS count
		     FROM user_job_state
		     WHERE user_id = ? AND status IS NOT NULL
		     GROUP BY status`, userID).
		Scan(&rows).Error; err != nil {
		return dto.Counts{}, fmt.Errorf("counts recompute workflow: %w", err)
	}
	out := dto.Counts{Saved: saved}
	for _, r := range rows {
		switch r.Status {
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

// seedCounter inserts a counter row for userID with the supplied counts.
// ON CONFLICT updates the existing row to the recomputed values (re-seeding
// is the rare path; we want consistency, not first-writer-wins).
func seedCounter(ctx context.Context, db *gorm.DB, userID string, c dto.Counts) error {
	row := jobmodel.UserJobCounters{
		UserID:            userID,
		SavedCount:        c.Saved,
		AppliedCount:      c.Applied,
		InterviewingCount: c.Interviewing,
		OfferCount:        c.Offer,
		RejectedCount:     c.Rejected,
		DismissedCount:    c.Dismissed,
		UpdatedAt:         time.Now(),
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"saved_count", "applied_count", "interviewing_count",
			"offer_count", "rejected_count", "dismissed_count", "updated_at",
		}),
	}).Create(&row).Error
}
