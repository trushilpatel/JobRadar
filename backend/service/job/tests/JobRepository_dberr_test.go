package services_test

import (
	"context"
	"testing"
	"time"

	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// --- UserJobStateRepository DB-error paths ---

// TestUserJobState_Get_DBError — line 70: Get returns a non-ErrRecordNotFound DB error.
func TestUserJobState_Get_DBError(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()

	repo := jobsvc.NewUserJobStateRepository(db)
	_, err := repo.Get(context.Background(), "u1", "j1")
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

// TestUserJobState_GetMany_DBError — line 91: GetMany returns a DB error.
func TestUserJobState_GetMany_DBError(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()

	repo := jobsvc.NewUserJobStateRepository(db)
	_, err := repo.GetMany(context.Background(), "u1", []string{"j1"})
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

// TestUserJobState_Upsert_InvalidStatus — line 189: Upsert default case
// returns BAD_REQUEST for an unrecognised status string.
func TestUserJobState_Upsert_InvalidStatus(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())

	repo := jobsvc.NewUserJobStateRepository(db)
	err := repo.Upsert(context.Background(), "u1", "j1", dto.UpsertState{Status: "bogus_status"})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

// TestUserJobState_CountsByUser_DBError — line 419: CountsByUser returns a
// non-ErrRecordNotFound error (closed connection, not "row not found").
func TestUserJobState_CountsByUser_DBError(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()

	repo := jobsvc.NewUserJobStateRepository(db)
	_, err := repo.CountsByUser(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

// TestUserJobState_SetSaved_False_RowHasAppliedAt — line 763:
// rowIsMeaningfulWithoutSaved returns true because AppliedAt is set on the row
// (no workflow status). Triggers the UPDATE path instead of DELETE.
func TestUserJobState_SetSaved_False_RowHasAppliedAt(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())

	// Insert a row with applied_at set but status explicitly cleared, so that
	// rowIsMeaningfulWithoutSaved falls through to the AppliedAt branch.
	now := time.Now()
	row := jobmodel.UserJobState{
		UserID:    "u1",
		JobID:     "j1",
		SavedAt:   &now,
		AppliedAt: &now,
		// Status is nil — mimics a partial state where status was cleared
		// but the applied timestamp was retained.
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed row: %v", err)
	}

	repo := jobsvc.NewUserJobStateRepository(db)
	if err := repo.SetSaved(context.Background(), "u1", "j1", false); err != nil {
		t.Fatalf("SetSaved false: %v", err)
	}

	// Row should still exist (appliedAt makes it meaningful without saved_at).
	var found jobmodel.UserJobState
	if err := db.Where("user_id = ? AND job_id = ?", "u1", "j1").First(&found).Error; err != nil {
		t.Fatalf("row missing after unsave: %v", err)
	}
	if found.SavedAt != nil {
		t.Error("SavedAt should have been cleared")
	}
}

// TestUserJobState_SetSaved_False_RowHasNotes — line 766:
// rowIsMeaningfulWithoutSaved returns true because Notes is set (no status,
// no appliedAt/dismissedAt).
func TestUserJobState_SetSaved_False_RowHasNotes(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())

	// Insert a row with notes set (no status, no applied/dismissed timestamps).
	now := time.Now()
	notes := "interesting role"
	row := jobmodel.UserJobState{
		UserID:  "u1",
		JobID:   "j1",
		SavedAt: &now,
		Notes:   &notes,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed row: %v", err)
	}

	repo := jobsvc.NewUserJobStateRepository(db)
	if err := repo.SetSaved(context.Background(), "u1", "j1", false); err != nil {
		t.Fatalf("SetSaved false: %v", err)
	}

	// Row should still exist because notes make it meaningful without saved_at.
	var found jobmodel.UserJobState
	if err := db.Where("user_id = ? AND job_id = ?", "u1", "j1").First(&found).Error; err != nil {
		t.Fatalf("row missing after unsave: %v", err)
	}
}

// --- JobRepositoryService DB-error paths ---

// TestJobStore_GetJobsByIDs_DBError — line 317: GetJobsByIDs returns a DB error.
func TestJobStore_GetJobsByIDs_DBError(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()

	store := jobsvc.NewJobStore(db)
	_, err := store.GetJobsByIDs(context.Background(), []string{"j1"})
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

// TestJobStore_InsertJob_DBError — line 345: InsertJob returns a DB error.
func TestJobStore_InsertJob_DBError(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()

	store := jobsvc.NewJobStore(db)
	err := store.InsertJob(context.Background(), &jobmodel.Job{
		ID:        "j1",
		JobPostID: "post-1",
		CompanyID: "c1",
		Title:     "SWE",
	})
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

// TestJobStore_GetDistinctCompanyNames_DBError — line 395: DB error in
// GetDistinctCompanyNames.
func TestJobStore_GetDistinctCompanyNames_DBError(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()

	store := jobsvc.NewJobStore(db)
	_, err := store.GetDistinctCompanyNames(context.Background())
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

// --- JobSeenLedgerStore DB-error paths ---

// TestSeenLedger_HasSeenBatch_DBError — line 58: returns error from closed DB.
func TestSeenLedger_HasSeenBatch_DBError(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()

	store := jobsvc.NewSeenLedgerStore(db)
	_, err := store.HasSeenBatch(context.Background(), []string{"j1"})
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

// TestSeenLedger_UpdateSeenReason_DBError — line 100: returns error from closed DB.
func TestSeenLedger_UpdateSeenReason_DBError(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()

	store := jobsvc.NewSeenLedgerStore(db)
	err := store.UpdateSeenReason(context.Background(), "j1", jobcommon.SeenReasonSaved)
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

// TestSeenLedger_UpdateSeenReasonBatch_DBError — line 119: returns error from
// closed DB.
func TestSeenLedger_UpdateSeenReasonBatch_DBError(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()

	store := jobsvc.NewSeenLedgerStore(db)
	err := store.UpdateSeenReasonBatch(context.Background(), []string{"j1"}, jobcommon.SeenReasonSaved)
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}
