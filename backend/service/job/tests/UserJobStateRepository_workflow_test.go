package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// TestUpsert_WorkflowStatuses exercises the interviewing/offer/rejected status
// branches in newSnapshot and the priorSnapshot switch cases by upsert-chaining
// each workflow status.
func TestUpsert_WorkflowStatuses(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	now := time.Now()
	for _, id := range []string{"j1", "j2", "j3", "j4", "j5", "j6", "j7", "j8"} {
		seedJob(t, db, id, "c1", "T", "p-"+id, now)
	}
	repo := jobsvc.NewUserJobStateRepository(db)
	ctx := context.Background()

	// Interviewing on new row — covers newSnapshot case StatusInterviewing.
	if err := repo.Upsert(ctx, "u1", "j1", dto.UpsertState{Status: jobmodel.StatusInterviewing}); err != nil {
		t.Fatalf("upsert interviewing: %v", err)
	}

	// Offer on new row — covers newSnapshot case StatusOffer.
	if err := repo.Upsert(ctx, "u1", "j2", dto.UpsertState{Status: jobmodel.StatusOffer}); err != nil {
		t.Fatalf("upsert offer: %v", err)
	}

	// Rejected on new row — covers newSnapshot case StatusRejected.
	if err := repo.Upsert(ctx, "u1", "j3", dto.UpsertState{Status: jobmodel.StatusRejected}); err != nil {
		t.Fatalf("upsert rejected: %v", err)
	}

	// Applied → Saved: priorSnapshot.applied + newSnapshot(saved+applied inner case).
	if err := repo.Upsert(ctx, "u1", "j4", dto.UpsertState{Status: jobmodel.StatusApplied}); err != nil {
		t.Fatalf("upsert applied j4: %v", err)
	}
	if err := repo.Upsert(ctx, "u1", "j4", dto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("upsert saved j4: %v", err)
	}

	// Interviewing → Saved: priorSnapshot.interviewing + newSnapshot(saved+interviewing).
	if err := repo.Upsert(ctx, "u1", "j5", dto.UpsertState{Status: jobmodel.StatusInterviewing}); err != nil {
		t.Fatalf("upsert interviewing j5: %v", err)
	}
	if err := repo.Upsert(ctx, "u1", "j5", dto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("upsert saved j5: %v", err)
	}

	// Offer → Saved: priorSnapshot.offer + newSnapshot(saved+offer).
	if err := repo.Upsert(ctx, "u1", "j6", dto.UpsertState{Status: jobmodel.StatusOffer}); err != nil {
		t.Fatalf("upsert offer j6: %v", err)
	}
	if err := repo.Upsert(ctx, "u1", "j6", dto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("upsert saved j6: %v", err)
	}

	// Rejected → Saved: priorSnapshot.rejected + newSnapshot(saved+rejected).
	if err := repo.Upsert(ctx, "u1", "j7", dto.UpsertState{Status: jobmodel.StatusRejected}); err != nil {
		t.Fatalf("upsert rejected j7: %v", err)
	}
	if err := repo.Upsert(ctx, "u1", "j7", dto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("upsert saved j7: %v", err)
	}

	// Dismissed → Saved: priorSnapshot.dismissed + newSnapshot(saved+dismissed).
	reason := "user_dismissed"
	if err := repo.Upsert(ctx, "u1", "j8", dto.UpsertState{Status: jobmodel.StatusDismissed, DismissalReason: &reason}); err != nil {
		t.Fatalf("upsert dismissed j8: %v", err)
	}
	if err := repo.Upsert(ctx, "u1", "j8", dto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("upsert saved j8: %v", err)
	}
}

// TestUpsert_StatusNewWithPrior covers the StatusNew branch in Upsert when a
// prior row already exists — lines 159/162/165 (SavedAt preserved, status cleared).
func TestUpsert_StatusNewWithPrior(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)
	ctx := context.Background()

	if err := repo.Upsert(ctx, "u1", "j1", dto.UpsertState{Status: jobmodel.StatusApplied}); err != nil {
		t.Fatalf("upsert applied: %v", err)
	}
	if err := repo.Upsert(ctx, "u1", "j1", dto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("upsert saved: %v", err)
	}
	// Now upsert(new) with prior row — covers lines 159, 162, 165.
	if err := repo.Upsert(ctx, "u1", "j1", dto.UpsertState{Status: "new"}); err != nil {
		t.Fatalf("upsert new: %v", err)
	}

	var row jobmodel.UserJobState
	if err := db.Where("user_id = ? AND job_id = ?", "u1", "j1").First(&row).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if row.Status != nil {
		t.Errorf("status = %q after new upsert, want nil", *row.Status)
	}
}

// TestUpsert_SavedWithPriorStatus covers line 155 — when Upsert(saved) runs
// with a prior row that has a workflow status, the status is copied to the row.
func TestUpsert_SavedWithPriorStatus(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)
	ctx := context.Background()

	if err := repo.Upsert(ctx, "u1", "j1", dto.UpsertState{Status: jobmodel.StatusApplied}); err != nil {
		t.Fatalf("upsert applied: %v", err)
	}
	if err := repo.Upsert(ctx, "u1", "j1", dto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("upsert saved: %v", err)
	}

	var row jobmodel.UserJobState
	if err := db.Where("user_id = ? AND job_id = ?", "u1", "j1").First(&row).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if row.StatusOrEmpty() != jobmodel.StatusApplied {
		t.Errorf("status = %q, want applied (prior preserved by saved upsert)", row.StatusOrEmpty())
	}
}

// TestUpsert_DismissedWithPriorSavedAt covers line 178 — prior SavedAt is
// preserved when the status is flipped to dismissed.
func TestUpsert_DismissedWithPriorSavedAt(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)
	ctx := context.Background()

	if err := repo.Upsert(ctx, "u1", "j1", dto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("upsert saved: %v", err)
	}
	first, _ := repo.Get(ctx, "u1", "j1")
	savedAt := first.SavedAt

	reason := "user_dismissed"
	if err := repo.Upsert(ctx, "u1", "j1", dto.UpsertState{Status: jobmodel.StatusDismissed, DismissalReason: &reason}); err != nil {
		t.Fatalf("upsert dismissed: %v", err)
	}

	second, err := repo.Get(ctx, "u1", "j1")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if second.SavedAt == nil || !second.SavedAt.Equal(*savedAt) {
		t.Errorf("SavedAt changed across dismiss: before=%v after=%v", savedAt, second.SavedAt)
	}
}

// TestDelete_WorkflowStatuses covers the sub() function branches — each workflow
// status that is removed must decrement its counter.
func TestDelete_WorkflowStatuses(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	now := time.Now()
	for _, id := range []string{"j1", "j2", "j3", "j4", "j5"} {
		seedJob(t, db, id, "c1", "T", "p-"+id, now)
	}
	repo := jobsvc.NewUserJobStateRepository(db)
	ctx := context.Background()

	reason := "user_dismissed"
	statuses := []struct {
		jobID  string
		status string
		reason *string
	}{
		{"j1", jobmodel.StatusApplied, nil},
		{"j2", jobmodel.StatusInterviewing, nil},
		{"j3", jobmodel.StatusOffer, nil},
		{"j4", jobmodel.StatusRejected, nil},
		{"j5", jobmodel.StatusDismissed, &reason},
	}
	for _, s := range statuses {
		st := dto.UpsertState{Status: s.status, DismissalReason: s.reason}
		if err := repo.Upsert(ctx, "u1", s.jobID, st); err != nil {
			t.Fatalf("upsert %s %s: %v", s.jobID, s.status, err)
		}
	}

	// Delete each row — sub() decrements the matching counter bucket.
	for _, s := range statuses {
		if err := repo.Delete(ctx, "u1", s.jobID); err != nil {
			t.Fatalf("delete %s: %v", s.jobID, err)
		}
	}

	var counter jobmodel.UserJobCounters
	if err := db.Where("user_id = ?", "u1").First(&counter).Error; err != nil {
		t.Fatalf("counter: %v", err)
	}
	if counter.AppliedCount != 0 || counter.InterviewingCount != 0 ||
		counter.OfferCount != 0 || counter.RejectedCount != 0 || counter.DismissedCount != 0 {
		t.Errorf("counters not zeroed after delete: %+v", counter)
	}
}

// TestCountsByUser_CounterRowPath — CountsByUser returns the cached row when
// user_job_counters has been populated by a prior Upsert (line 409 branch).
func TestCountsByUser_CounterRowPath(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)
	ctx := context.Background()

	// Upsert creates the user_job_counters row.
	if err := repo.Upsert(ctx, "u1", "j1", dto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	counts, err := repo.CountsByUser(ctx, "u1")
	if err != nil {
		t.Fatalf("CountsByUser: %v", err)
	}
	if counts.Saved != 1 {
		t.Errorf("saved = %d, want 1 (read from counter row)", counts.Saved)
	}
}

// TestUpsert_AppliedToInterviewing_BoolDeltaMinus1 — going from applied to
// interviewing decrements the applied counter, covering boolDelta return -1.
func TestUpsert_AppliedToInterviewing_BoolDeltaMinus1(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)
	ctx := context.Background()

	if err := repo.Upsert(ctx, "u1", "j1", dto.UpsertState{Status: jobmodel.StatusApplied}); err != nil {
		t.Fatalf("upsert applied: %v", err)
	}
	if err := repo.Upsert(ctx, "u1", "j1", dto.UpsertState{Status: jobmodel.StatusInterviewing}); err != nil {
		t.Fatalf("upsert interviewing: %v", err)
	}

	counts, err := repo.CountsByUser(ctx, "u1")
	if err != nil {
		t.Fatalf("CountsByUser: %v", err)
	}
	if counts.Applied != 0 {
		t.Errorf("applied = %d, want 0 after flip to interviewing", counts.Applied)
	}
	if counts.Interviewing != 1 {
		t.Errorf("interviewing = %d, want 1", counts.Interviewing)
	}
}
