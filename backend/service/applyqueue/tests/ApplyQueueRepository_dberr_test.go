package tests

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	applyqueuecommon "github.com/trushilpatel/jobradar/service/applyqueue/common"
	applyqueuesvc "github.com/trushilpatel/jobradar/service/applyqueue/services"
)

func TestApplyQueueRepo_List_DBError(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	_ = sqlDB.Close()

	repo := applyqueuesvc.NewRepositoryService(db)
	if _, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20); err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestApplyQueueRepo_SetResume_Clear_DBError(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	_ = sqlDB.Close()

	repo := applyqueuesvc.NewRepositoryService(db)
	if _, err := repo.SetResume(context.Background(), testUserID, "j1", nil); err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestApplyQueueRepo_SetResume_Attach_UpsertDBError(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	_ = sqlDB.Close()

	repo := applyqueuesvc.NewRepositoryService(db)
	resumeID := "r1"
	if _, err := repo.SetResume(context.Background(), testUserID, "j1", &resumeID); err == nil {
		t.Fatal("expected error on closed DB")
	}
}

// TestApplyQueueRepo_SetResume_Attach_LabelFetchDBError covers the branch
// where the upsert into resume_job_attachments succeeds (no real FK
// enforcement in the sqlite test schema) but the follow-up label lookup
// fails — simulated by dropping the resumes table after the attachment
// write path no longer needs it.
func TestApplyQueueRepo_SetResume_Attach_LabelFetchDBError(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	if err := db.Exec("DROP TABLE resumes").Error; err != nil {
		t.Fatalf("drop resumes: %v", err)
	}

	repo := applyqueuesvc.NewRepositoryService(db)
	resumeID := "r1"
	view, err := repo.SetResume(context.Background(), testUserID, "j1", &resumeID)
	if err == nil {
		t.Fatal("expected error when resumes table is missing")
	}
	if view != nil {
		t.Fatalf("expected nil view on error, got %+v", view)
	}
}

// TestApplyQueueRepo_List_JobKeywordsQueryDBError forces attachJobKeywords'
// job_tech_tags lookup to fail (base query already succeeded — there's a row
// to attach keywords to) and asserts List surfaces it as an error rather than
// silently returning items with no keywords.
func TestApplyQueueRepo_List_JobKeywordsQueryDBError(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)

	if err := db.Exec("DROP TABLE job_tech_tags").Error; err != nil {
		t.Fatalf("drop job_tech_tags: %v", err)
	}

	repo := applyqueuesvc.NewRepositoryService(db)
	if _, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20); err == nil {
		t.Fatal("expected an error when the job_tech_tags lookup fails")
	}
}

func TestApplyQueueRepo_HasEngagement_DBError(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	_ = sqlDB.Close()

	checker := applyqueuesvc.NewEngagementChecker(db)
	if _, err := checker.HasEngagement(context.Background(), testUserID, "j1"); err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestApplyQueueRepo_ResumeOwnedByUser_DBError(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	_ = sqlDB.Close()

	checker := applyqueuesvc.NewResumeOwnershipChecker(db)
	if _, err := checker.ResumeOwnedByUser(context.Background(), testUserID, "r1"); err == nil {
		t.Fatal("expected error on closed DB")
	}
}

// TestApplyQueueRepo_List_InvalidBase64Cursor_RestartsFromFirstPage covers
// decodeCursor's base64-decode-failure branch specifically (distinct from
// the "valid base64, invalid JSON" case already covered by
// TestApplyQueueRepo_List_InvalidCursor_RestartsFromFirstPage) — "!" is not
// in the base64 URL-safe alphabet.
func TestApplyQueueRepo_List_InvalidBase64Cursor_RestartsFromFirstPage(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	now := time.Now()
	seedUserJobState(t, db, testUserID, "j1", nil, &now, now)

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "!!!not-base64!!!", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1 (invalid base64 cursor should restart at page 1)", len(result.Items))
	}
}

// TestApplyQueueRepo_List_CursorWithEmptyJobID_RestartsFromFirstPage covers
// decodeCursor's "valid JSON, empty job id" branch — a cursor that decodes
// cleanly but carries no tiebreaker is treated as corrupt/unusable.
func TestApplyQueueRepo_List_CursorWithEmptyJobID_RestartsFromFirstPage(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	now := time.Now()
	seedUserJobState(t, db, testUserID, "j1", nil, &now, now)

	raw, _ := json.Marshal(struct {
		U int64  `json:"u"`
		J string `json:"j"`
	}{U: 123, J: ""})
	cursor := base64.RawURLEncoding.EncodeToString(raw)

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, cursor, 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1 (empty job id in cursor should restart at page 1)", len(result.Items))
	}
}
