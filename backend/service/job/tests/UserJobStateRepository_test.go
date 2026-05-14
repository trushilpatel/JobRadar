package services_test

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// TestUserJobStateRepo_Get_HappyPath inserts a row via the repo, reads it
// back, and checks every field is preserved.
func TestUserJobStateRepo_Get_HappyPath(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)
	if err := repo.Upsert(context.Background(), "u1", "j1", dto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := repo.Get(context.Background(), "u1", "j1")
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	if got.Status != jobmodel.StatusSaved {
		t.Errorf("status = %q, want saved", got.Status)
	}
	if got.SavedAt == nil {
		t.Errorf("SavedAt not populated")
	}
}

// TestUserJobStateRepo_Get_NotFound returns the typed NOT_FOUND error.
func TestUserJobStateRepo_Get_NotFound(t *testing.T) {
	t.Parallel()
	repo := jobsvc.NewUserJobStateRepository(newJobTestDB(t))
	_, err := repo.Get(context.Background(), "u1", "j-missing")
	var serr *apperr.ServiceError
	if !stderrors.As(err, &serr) || serr.Code != "NOT_FOUND" {
		t.Fatalf("err = %v, want NOT_FOUND", err)
	}
}

// TestUserJobStateRepo_GetMany_Empty returns an empty map (not nil) when the
// jobIDs slice is empty.
func TestUserJobStateRepo_GetMany_Empty(t *testing.T) {
	t.Parallel()
	repo := jobsvc.NewUserJobStateRepository(newJobTestDB(t))
	out, err := repo.GetMany(context.Background(), "u1", nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out == nil {
		t.Fatal("nil map; want empty")
	}
	if len(out) != 0 {
		t.Errorf("len = %d, want 0", len(out))
	}
}

// TestUserJobStateRepo_GetMany_KeyedByJobID returns the matching rows keyed
// by JobID; absent ones are simply not in the map.
func TestUserJobStateRepo_GetMany_KeyedByJobID(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	now := time.Now()
	seedJob(t, db, "j1", "c1", "T", "p1", now)
	seedJob(t, db, "j2", "c1", "T", "p2", now)
	seedJob(t, db, "j3", "c1", "T", "p3", now)
	repo := jobsvc.NewUserJobStateRepository(db)
	if err := repo.Upsert(context.Background(), "u1", "j1", dto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("upsert j1: %v", err)
	}
	if err := repo.Upsert(context.Background(), "u1", "j3", dto.UpsertState{Status: jobmodel.StatusApplied}); err != nil {
		t.Fatalf("upsert j3: %v", err)
	}

	out, err := repo.GetMany(context.Background(), "u1", []string{"j1", "j2", "j3"})
	if err != nil {
		t.Fatalf("GetMany err = %v", err)
	}
	if out["j1"] == nil || out["j1"].Status != jobmodel.StatusSaved {
		t.Errorf("j1 = %+v", out["j1"])
	}
	if out["j3"] == nil || out["j3"].Status != jobmodel.StatusApplied {
		t.Errorf("j3 = %+v", out["j3"])
	}
	if out["j2"] != nil {
		t.Errorf("j2 present in map; want absent (no row)")
	}
}

// TestUserJobStateRepo_GetMany_UserScoped — rows for a different user must
// not leak into this user's overlay.
func TestUserJobStateRepo_GetMany_UserScoped(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)
	if err := repo.Upsert(context.Background(), "u-other", "j1", dto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("seed other user: %v", err)
	}
	out, err := repo.GetMany(context.Background(), "u1", []string{"j1"})
	if err != nil {
		t.Fatalf("GetMany: %v", err)
	}
	if _, ok := out["j1"]; ok {
		t.Errorf("u1 should see no row for j1 — got %+v", out["j1"])
	}
}

// TestUserJobStateRepo_Upsert_RejectsEmptyJobID returns BAD_REQUEST when the
// job ID is empty (validation before the DB write).
func TestUserJobStateRepo_Upsert_RejectsEmptyJobID(t *testing.T) {
	t.Parallel()
	repo := jobsvc.NewUserJobStateRepository(newJobTestDB(t))
	err := repo.Upsert(context.Background(), "u1", "", dto.UpsertState{Status: jobmodel.StatusSaved})
	var serr *apperr.ServiceError
	if !stderrors.As(err, &serr) || serr.Code != "BAD_REQUEST" {
		t.Fatalf("err = %v, want BAD_REQUEST", err)
	}
}

// TestUserJobStateRepo_Upsert_RejectsEmptyStatus returns BAD_REQUEST when
// the status is empty (validation before DB).
func TestUserJobStateRepo_Upsert_RejectsEmptyStatus(t *testing.T) {
	t.Parallel()
	repo := jobsvc.NewUserJobStateRepository(newJobTestDB(t))
	err := repo.Upsert(context.Background(), "u1", "j1", dto.UpsertState{Status: ""})
	var serr *apperr.ServiceError
	if !stderrors.As(err, &serr) || serr.Code != "BAD_REQUEST" {
		t.Fatalf("err = %v, want BAD_REQUEST", err)
	}
}

// TestUserJobStateRepo_Upsert_StatusFlipPreservesPriorTimestamps — saving
// then applying must NOT null saved_at when status flips to applied; only
// the active-status timestamp column is updated on conflict.
func TestUserJobStateRepo_Upsert_StatusFlipPreservesPriorTimestamps(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)

	if err := repo.Upsert(context.Background(), "u1", "j1", dto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("save: %v", err)
	}
	first, err := repo.Get(context.Background(), "u1", "j1")
	if err != nil {
		t.Fatalf("read after save: %v", err)
	}
	savedAtBefore := first.SavedAt

	// Flip to applied — saved_at must be preserved, applied_at populated.
	if err := repo.Upsert(context.Background(), "u1", "j1", dto.UpsertState{Status: jobmodel.StatusApplied}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	second, err := repo.Get(context.Background(), "u1", "j1")
	if err != nil {
		t.Fatalf("read after apply: %v", err)
	}
	if second.SavedAt == nil || !second.SavedAt.Equal(*savedAtBefore) {
		t.Errorf("saved_at changed across status flip: before=%v after=%v", savedAtBefore, second.SavedAt)
	}
	if second.AppliedAt == nil {
		t.Errorf("applied_at nil after apply")
	}
	if second.Status != jobmodel.StatusApplied {
		t.Errorf("status = %q, want applied", second.Status)
	}
}

// TestUserJobStateRepo_Upsert_DismissalSetsAllDismissalCols — a dismissed
// upsert sets dismissed_at, dismissal_reason, dismissal_note.
func TestUserJobStateRepo_Upsert_DismissalSetsAllDismissalCols(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)
	reason := "user_dismissed"
	note := "stale"
	if err := repo.Upsert(context.Background(), "u1", "j1", dto.UpsertState{
		Status:          jobmodel.StatusDismissed,
		DismissalReason: &reason,
		DismissalNote:   &note,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	row, err := repo.Get(context.Background(), "u1", "j1")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if row.DismissedAt == nil {
		t.Errorf("dismissed_at nil")
	}
	if row.DismissalReason == nil || *row.DismissalReason != reason {
		t.Errorf("dismissal_reason = %v, want %q", row.DismissalReason, reason)
	}
	if row.DismissalNote == nil || *row.DismissalNote != note {
		t.Errorf("dismissal_note = %v, want %q", row.DismissalNote, note)
	}
}

// TestUserJobStateRepo_Upsert_WithNotes — notes pointer is forwarded to the
// updateCols list and persisted.
func TestUserJobStateRepo_Upsert_WithNotes(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)
	notes := "my note"
	if err := repo.Upsert(context.Background(), "u1", "j1", dto.UpsertState{
		Status: jobmodel.StatusSaved,
		Notes:  &notes,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	row, err := repo.Get(context.Background(), "u1", "j1")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if row.Notes == nil || *row.Notes != notes {
		t.Errorf("notes = %v, want %q", row.Notes, notes)
	}
}

// TestUserJobStateRepo_UpsertBulkDismiss_Empty no-ops.
func TestUserJobStateRepo_UpsertBulkDismiss_Empty(t *testing.T) {
	t.Parallel()
	repo := jobsvc.NewUserJobStateRepository(newJobTestDB(t))
	if err := repo.UpsertBulkDismiss(context.Background(), "u1", nil, "user_dismissed", ""); err != nil {
		t.Errorf("empty batch should no-op: %v", err)
	}
}

// TestUserJobStateRepo_UpsertBulkDismiss_NewRows inserts one dismissed row
// per ID. Note pointer is nil when the note is empty.
func TestUserJobStateRepo_UpsertBulkDismiss_NewRows(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	now := time.Now()
	seedJob(t, db, "j1", "c1", "T", "p1", now)
	seedJob(t, db, "j2", "c1", "T", "p2", now)
	repo := jobsvc.NewUserJobStateRepository(db)
	if err := repo.UpsertBulkDismiss(context.Background(), "u1", []string{"j1", "j2"}, "user_not_relevant", ""); err != nil {
		t.Fatalf("bulk dismiss: %v", err)
	}
	out, err := repo.GetMany(context.Background(), "u1", []string{"j1", "j2"})
	if err != nil {
		t.Fatalf("GetMany: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	for id, row := range out {
		if row.Status != jobmodel.StatusDismissed {
			t.Errorf("%s status = %q, want dismissed", id, row.Status)
		}
		if row.DismissedAt == nil {
			t.Errorf("%s dismissed_at nil", id)
		}
		if row.DismissalReason == nil || *row.DismissalReason != "user_not_relevant" {
			t.Errorf("%s reason = %v", id, row.DismissalReason)
		}
		if row.DismissalNote != nil {
			t.Errorf("%s note = %v, want nil (empty note)", id, row.DismissalNote)
		}
	}
}

// TestUserJobStateRepo_UpsertBulkDismiss_WithNote — non-empty note pointer.
func TestUserJobStateRepo_UpsertBulkDismiss_WithNote(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)
	if err := repo.UpsertBulkDismiss(context.Background(), "u1", []string{"j1"}, "user_dismissed", "boom"); err != nil {
		t.Fatalf("bulk dismiss: %v", err)
	}
	row, err := repo.Get(context.Background(), "u1", "j1")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if row.DismissalNote == nil || *row.DismissalNote != "boom" {
		t.Errorf("note = %v", row.DismissalNote)
	}
}

// TestUserJobStateRepo_Delete_Existing — Delete removes the (user, job)
// row; a subsequent Get returns NOT_FOUND.
func TestUserJobStateRepo_Delete_Existing(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)
	if err := repo.Upsert(context.Background(), "u1", "j1", dto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := repo.Delete(context.Background(), "u1", "j1"); err != nil {
		t.Fatalf("Delete err = %v", err)
	}
	_, err := repo.Get(context.Background(), "u1", "j1")
	var serr *apperr.ServiceError
	if !stderrors.As(err, &serr) || serr.Code != "NOT_FOUND" {
		t.Errorf("post-delete Get = %v, want NOT_FOUND", err)
	}
}

// TestUserJobStateRepo_Delete_NonExistent — deleting a missing row is a
// no-op (GORM Delete with WHERE returns no error).
func TestUserJobStateRepo_Delete_NonExistent(t *testing.T) {
	t.Parallel()
	repo := jobsvc.NewUserJobStateRepository(newJobTestDB(t))
	if err := repo.Delete(context.Background(), "u1", "ghost"); err != nil {
		t.Errorf("delete missing: %v", err)
	}
}

// TestUserJobStateRepo_CountsByUser groups rows by status. Statuses not
// represented stay zero. Verifies every branch in the switch.
func TestUserJobStateRepo_CountsByUser(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	now := time.Now()
	for i, st := range []string{
		jobmodel.StatusSaved, jobmodel.StatusSaved, jobmodel.StatusSaved, // 3 saved
		jobmodel.StatusApplied, jobmodel.StatusApplied, // 2 applied
		jobmodel.StatusDismissed, // 1 dismissed
		jobmodel.StatusInterviewing,
		jobmodel.StatusOffer,
		jobmodel.StatusRejected,
	} {
		jid := jobID(i)
		seedJob(t, db, jid, "c1", "T", "p", now)
		if err := db.Create(&jobmodel.UserJobState{
			UserID: "u1", JobID: jid, Status: st,
		}).Error; err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	repo := jobsvc.NewUserJobStateRepository(db)
	counts, err := repo.CountsByUser(context.Background(), "u1")
	if err != nil {
		t.Fatalf("CountsByUser: %v", err)
	}
	if counts.Saved != 3 || counts.Applied != 2 || counts.Dismissed != 1 ||
		counts.Interviewing != 1 || counts.Offer != 1 || counts.Rejected != 1 {
		t.Errorf("counts = %+v", counts)
	}
}

// TestUserJobStateRepo_CountsByUser_EmptyZeroAll — zero rows → all zeros.
func TestUserJobStateRepo_CountsByUser_EmptyZeroAll(t *testing.T) {
	t.Parallel()
	repo := jobsvc.NewUserJobStateRepository(newJobTestDB(t))
	counts, err := repo.CountsByUser(context.Background(), "u1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if (counts != dto.Counts{}) {
		t.Errorf("counts = %+v, want zero", counts)
	}
}
