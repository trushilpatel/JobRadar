package services_test

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

func TestUserJobState_SetSaved_EmptyUserID(t *testing.T) {
	t.Parallel()
	repo := jobsvc.NewUserJobStateRepository(newJobTestDB(t))
	err := repo.SetSaved(context.Background(), "", "j1", true)
	var se *apperr.ServiceError
	if !stderrors.As(err, &se) || se.Code != "INTERNAL" {
		t.Fatalf("err = %v, want INTERNAL", err)
	}
}

func TestUserJobState_SetSaved_EmptyJobID(t *testing.T) {
	t.Parallel()
	repo := jobsvc.NewUserJobStateRepository(newJobTestDB(t))
	err := repo.SetSaved(context.Background(), "u1", "", true)
	var se *apperr.ServiceError
	if !stderrors.As(err, &se) || se.Code != "BAD_REQUEST" {
		t.Fatalf("err = %v, want BAD_REQUEST", err)
	}
}

// TestUserJobState_SetSaved_TrueNewRow — inserts a saved row and seeds the counter.
func TestUserJobState_SetSaved_TrueNewRow(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)

	if err := repo.SetSaved(context.Background(), "u1", "j1", true); err != nil {
		t.Fatalf("SetSaved err = %v", err)
	}

	var row jobmodel.UserJobState
	if err := db.Where("user_id = ? AND job_id = ?", "u1", "j1").First(&row).Error; err != nil {
		t.Fatalf("row missing: %v", err)
	}
	if row.SavedAt == nil {
		t.Error("SavedAt should be set")
	}
	var counter jobmodel.UserJobCounters
	if err := db.Where("user_id = ?", "u1").First(&counter).Error; err != nil {
		t.Fatalf("counter missing: %v", err)
	}
	if counter.SavedCount != 1 {
		t.Errorf("saved_count = %d, want 1", counter.SavedCount)
	}
}

// TestUserJobState_SetSaved_TrueAlreadySaved — idempotent; counter stays at 1.
func TestUserJobState_SetSaved_TrueAlreadySaved(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)

	for range 2 {
		if err := repo.SetSaved(context.Background(), "u1", "j1", true); err != nil {
			t.Fatalf("SetSaved: %v", err)
		}
	}

	var counter jobmodel.UserJobCounters
	if err := db.Where("user_id = ?", "u1").First(&counter).Error; err != nil {
		t.Fatalf("counter: %v", err)
	}
	if counter.SavedCount != 1 {
		t.Errorf("saved_count = %d, want 1 (idempotent)", counter.SavedCount)
	}
}

// TestUserJobState_SetSaved_TruePriorRowHasStatus — prior workflow status is
// preserved when saved=true is applied (uses the prior.Status copy branch).
func TestUserJobState_SetSaved_TruePriorRowHasStatus(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)

	if err := repo.Upsert(context.Background(), "u1", "j1", dto.UpsertState{Status: jobmodel.StatusApplied}); err != nil {
		t.Fatalf("upsert applied: %v", err)
	}
	if err := repo.SetSaved(context.Background(), "u1", "j1", true); err != nil {
		t.Fatalf("SetSaved: %v", err)
	}

	var row jobmodel.UserJobState
	if err := db.Where("user_id = ? AND job_id = ?", "u1", "j1").First(&row).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if row.SavedAt == nil {
		t.Error("SavedAt not set")
	}
	if row.StatusOrEmpty() != jobmodel.StatusApplied {
		t.Errorf("status = %q, want applied (prior preserved)", row.StatusOrEmpty())
	}
}

// TestUserJobState_SetSaved_FalseNoPriorRow — no-op when no row exists.
func TestUserJobState_SetSaved_FalseNoPriorRow(t *testing.T) {
	t.Parallel()
	repo := jobsvc.NewUserJobStateRepository(newJobTestDB(t))
	if err := repo.SetSaved(context.Background(), "u1", "no-such-job", false); err != nil {
		t.Fatalf("SetSaved false no-op: %v", err)
	}
}

// TestUserJobState_SetSaved_FalseRowHasStatus — row has a workflow status so
// saved=false must UPDATE (clear saved_at) rather than DELETE the row.
func TestUserJobState_SetSaved_FalseRowHasStatus(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)

	if err := repo.Upsert(context.Background(), "u1", "j1", dto.UpsertState{Status: jobmodel.StatusApplied}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := repo.SetSaved(context.Background(), "u1", "j1", true); err != nil {
		t.Fatalf("SetSaved true: %v", err)
	}
	if err := repo.SetSaved(context.Background(), "u1", "j1", false); err != nil {
		t.Fatalf("SetSaved false: %v", err)
	}

	var row jobmodel.UserJobState
	if err := db.Where("user_id = ? AND job_id = ?", "u1", "j1").First(&row).Error; err != nil {
		t.Fatalf("row should exist (meaningful without saved): %v", err)
	}
	if row.SavedAt != nil {
		t.Error("SavedAt should be nil after SetSaved(false)")
	}
	if row.StatusOrEmpty() != jobmodel.StatusApplied {
		t.Errorf("status = %q, want applied", row.StatusOrEmpty())
	}
}

// TestUserJobState_SetSaved_FalseOnlySaved — row has only saved_at; saved=false
// must DELETE the row (nothing else left, non_empty constraint would fire).
func TestUserJobState_SetSaved_FalseOnlySaved(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)

	if err := repo.SetSaved(context.Background(), "u1", "j1", true); err != nil {
		t.Fatalf("SetSaved true: %v", err)
	}
	if err := repo.SetSaved(context.Background(), "u1", "j1", false); err != nil {
		t.Fatalf("SetSaved false: %v", err)
	}

	var count int64
	db.Model(&jobmodel.UserJobState{}).Where("user_id = ? AND job_id = ?", "u1", "j1").Count(&count)
	if count != 0 {
		t.Errorf("row count = %d, want 0 (deleted when only saved_at was set)", count)
	}
	var counter jobmodel.UserJobCounters
	if err := db.Where("user_id = ?", "u1").First(&counter).Error; err != nil {
		t.Fatalf("counter: %v", err)
	}
	if counter.SavedCount != 0 {
		t.Errorf("saved_count = %d, want 0 after unsave", counter.SavedCount)
	}
}

// TestUserJobState_NewRepo_WithLogger — exercises the `l = log[0]` branch in
// NewUserJobStateRepository when an explicit logger is supplied.
func TestUserJobState_NewRepo_WithLogger(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db, logger.NewNoop())
	if err := repo.SetSaved(context.Background(), "u1", "j1", true); err != nil {
		t.Fatalf("SetSaved with explicit logger: %v", err)
	}
}
