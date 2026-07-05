package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	applyqueuecommon "github.com/trushilpatel/jobradar/service/applyqueue/common"
	applyqueuesvc "github.com/trushilpatel/jobradar/service/applyqueue/services"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// seedUserJobState inserts a user_job_state row then forces updated_at to an
// explicit value via raw UPDATE — GORM's autoUpdateTime tag overwrites any
// value set before Create, so a raw follow-up write is the only way to get
// deterministic, distinct timestamps for cursor-ordering tests.
func seedUserJobState(t *testing.T, db *gorm.DB, userID, jobID string, status *string, savedAt *time.Time, updatedAt time.Time) {
	t.Helper()
	row := jobmodel.UserJobState{UserID: userID, JobID: jobID, Status: status, SavedAt: savedAt}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed user_job_state (%s,%s): %v", userID, jobID, err)
	}
	if err := db.Exec("UPDATE user_job_state SET updated_at = ? WHERE user_id = ? AND job_id = ?",
		updatedAt, userID, jobID).Error; err != nil {
		t.Fatalf("force updated_at (%s,%s): %v", userID, jobID, err)
	}
}

func strPtr(s string) *string { return &s }

const testUserID = "u1"

func TestApplyQueueRepo_List_FilterSaved_ReturnsOnlySavedRows(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	seedJob(t, db, "j2", "c1", "Designer")
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)                                            // saved only
	seedUserJobState(t, db, testUserID, "j2", strPtr(jobcommon.JobStatusApplied), nil, now.Add(time.Second)) // applied, not saved

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterSaved, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].JobID != "j1" {
		t.Fatalf("items = %+v, want exactly [j1]", result.Items)
	}
	if result.Items[0].Status != jobcommon.JobStatusSaved {
		t.Errorf("status = %q, want %q", result.Items[0].Status, jobcommon.JobStatusSaved)
	}
}

func TestApplyQueueRepo_List_FilterAll_IncludesSavedAppliedInterviewing(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	seedJob(t, db, "j2", "c1", "Designer")
	seedJob(t, db, "j3", "c1", "PM")
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)
	seedUserJobState(t, db, testUserID, "j2", strPtr(jobcommon.JobStatusApplied), nil, now.Add(time.Second))
	seedUserJobState(t, db, testUserID, "j3", strPtr(jobcommon.JobStatusInterviewing), nil, now.Add(2*time.Second))

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("len(items) = %d, want 3: %+v", len(result.Items), result.Items)
	}
}

// TestApplyQueueRepo_List_FilterAll_ExcludesDismissedOfferRejected covers the
// straightforward exclusion: a job with a terminal status and no saved_at
// never appears in the "all" queue.
func TestApplyQueueRepo_List_FilterAll_ExcludesDismissedOfferRejected(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	seedJob(t, db, "j2", "c1", "Designer")
	seedJob(t, db, "j3", "c1", "PM")
	now := time.Now()
	seedUserJobState(t, db, testUserID, "j1", strPtr(jobcommon.JobStatusDismissed), nil, now)
	seedUserJobState(t, db, testUserID, "j2", strPtr(jobcommon.JobStatusOffer), nil, now.Add(time.Second))
	seedUserJobState(t, db, testUserID, "j3", strPtr(jobcommon.JobStatusRejected), nil, now.Add(2*time.Second))

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("len(items) = %d, want 0: %+v", len(result.Items), result.Items)
	}
}

// TestApplyQueueRepo_List_FilterAll_SavedThenDismissed_Excluded documents the
// deliberate deviation from the spec's literal WHERE clause: saved_at
// survives a dismiss (UserJobStateRepository.Upsert preserves it), so a job
// saved and later dismissed would match "saved_at IS NOT NULL" — but the
// spec explicitly says dismissed jobs must be excluded from "all". The
// repo's extra exclusion guard wins.
func TestApplyQueueRepo_List_FilterAll_SavedThenDismissed_Excluded(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", strPtr(jobcommon.JobStatusDismissed), &savedAt, now)

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("len(items) = %d, want 0 (saved-then-dismissed must be excluded from all): %+v", len(result.Items), result.Items)
	}
}

// TestApplyQueueRepo_List_FilterSaved_ExcludesDismissedEvenWhenSaved documents
// that the terminal-status exclusion applies to both filters, not just
// "all" — a saved-then-dismissed job never appears in the queue under any
// filter, and consequently deriveStatus's output is always restricted to
// {saved, applied, interviewing}, matching the frontend's
// ApplyQueueStatusSchema three-value union.
func TestApplyQueueRepo_List_FilterSaved_ExcludesDismissedEvenWhenSaved(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", strPtr(jobcommon.JobStatusDismissed), &savedAt, now)

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterSaved, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("len(items) = %d, want 0 (saved-then-dismissed excluded from saved filter too): %+v", len(result.Items), result.Items)
	}
}

func TestApplyQueueRepo_List_ScopedToUser_DoesNotLeakOtherUsersRows(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, "other-user", "j1", nil, &savedAt, now)

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("len(items) = %d, want 0 (must not see other user's rows): %+v", len(result.Items), result.Items)
	}
}

func TestApplyQueueRepo_List_IncludesAttachedResume(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	seedResume(t, db, "r1", testUserID, "My Resume")
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)

	repo := applyqueuesvc.NewRepositoryService(db)
	resumeID := "r1"
	if _, err := repo.SetResume(context.Background(), testUserID, "j1", &resumeID); err != nil {
		t.Fatalf("SetResume err = %v", err)
	}

	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(result.Items))
	}
	item := result.Items[0]
	if item.ResumeID == nil || *item.ResumeID != "r1" {
		t.Errorf("ResumeID = %v, want r1", item.ResumeID)
	}
	if item.ResumeLabel == nil || *item.ResumeLabel != "My Resume" {
		t.Errorf("ResumeLabel = %v, want %q", item.ResumeLabel, "My Resume")
	}
}

func TestApplyQueueRepo_List_NoAttachment_ResumeFieldsNil(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(result.Items))
	}
	if result.Items[0].ResumeID != nil || result.Items[0].ResumeLabel != nil {
		t.Errorf("expected nil resume fields, got id=%v label=%v", result.Items[0].ResumeID, result.Items[0].ResumeLabel)
	}
}

// ─── Job detail fields (Location/Remote/SalaryRange/AddedAt) ────────────────

// TestApplyQueueRepo_List_JobDetailFields_NullLocationAndSalary_NoPanic covers
// the common case: seedJob leaves jobs.location and jobs.salary_range NULL
// (a job the pipeline never parsed a location/salary for). The repo must not
// panic scanning these into QueueItem, Location must surface as a nil
// pointer, and SalaryRange must marshal to valid JSON (`null`), never a
// malformed byte sequence.
func TestApplyQueueRepo_List_JobDetailFields_NullLocationAndSalary_NoPanic(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	before := time.Now().Add(-time.Second)
	seedJob(t, db, "j1", "c1", "Engineer") // no location/remote/salary_range set — all zero values
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(result.Items))
	}
	item := result.Items[0]

	if item.Location != nil {
		t.Errorf("Location = %v, want nil for a job with no parsed location", *item.Location)
	}
	if item.Remote != false {
		t.Errorf("Remote = %v, want false (default)", item.Remote)
	}

	if item.SalaryRange != nil {
		t.Errorf("SalaryRange = %q, want nil for a job with no salary data", string(item.SalaryRange))
	}
	// Confirm the whole item still marshals to valid, non-malformed JSON with
	// salary_range rendered as a JSON null — not omitted, not empty bytes.
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal QueueItem with nil SalaryRange: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("marshalled QueueItem is not valid JSON: %v (raw=%s)", err, raw)
	}
	if string(decoded["salary_range"]) != "null" {
		t.Errorf("salary_range in marshalled JSON = %s, want null", decoded["salary_range"])
	}
	if string(decoded["location"]) != "null" {
		t.Errorf("location in marshalled JSON = %s, want null", decoded["location"])
	}

	if item.AddedAt.IsZero() {
		t.Error("AddedAt is zero, want the job's autoCreateTime timestamp")
	}
	if item.AddedAt.Before(before) {
		t.Errorf("AddedAt = %v, want at/after test start %v", item.AddedAt, before)
	}
}

// TestApplyQueueRepo_List_JobDetailFields_PopulatedValues covers the
// populated case: a job with location/remote/salary_range all set must
// pass those values through to QueueItem unchanged.
func TestApplyQueueRepo_List_JobDetailFields_PopulatedValues(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	location := "Remote - US"
	salary := datatypes.JSON(`{"min":100000,"max":150000}`)
	seedJobWithDetails(t, db, "j1", "c1", "Engineer", &location, true, salary)
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(result.Items))
	}
	item := result.Items[0]

	if item.Location == nil || *item.Location != "Remote - US" {
		t.Errorf("Location = %v, want %q", item.Location, "Remote - US")
	}
	if item.Remote != true {
		t.Errorf("Remote = %v, want true", item.Remote)
	}

	var got, want map[string]interface{}
	if err := json.Unmarshal(item.SalaryRange, &got); err != nil {
		t.Fatalf("SalaryRange not valid JSON: %v (raw=%q)", err, string(item.SalaryRange))
	}
	_ = json.Unmarshal(salary, &want)
	if got["min"] != want["min"] || got["max"] != want["max"] {
		t.Errorf("SalaryRange = %v, want %v", got, want)
	}

	if item.AddedAt.IsZero() {
		t.Error("AddedAt is zero, want the job's autoCreateTime timestamp")
	}
}

func TestApplyQueueRepo_List_Pagination_CursorAdvances(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	now := time.Now()
	for i, id := range []string{"j1", "j2", "j3"} {
		seedJob(t, db, id, "c1", "Job "+id)
		savedAt := now.Add(time.Duration(i) * time.Second)
		seedUserJobState(t, db, testUserID, id, nil, &savedAt, now.Add(time.Duration(i)*time.Second))
	}

	repo := applyqueuesvc.NewRepositoryService(db)
	page1, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 2)
	if err != nil {
		t.Fatalf("page1 err = %v", err)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1.Items))
	}
	// Order is updated_at DESC — j3 (latest) then j2.
	if page1.Items[0].JobID != "j3" || page1.Items[1].JobID != "j2" {
		t.Fatalf("page1 order = [%s,%s], want [j3,j2]", page1.Items[0].JobID, page1.Items[1].JobID)
	}
	if page1.NextCursor == "" {
		t.Fatal("expected a next cursor for page1")
	}

	page2, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, page1.NextCursor, 2)
	if err != nil {
		t.Fatalf("page2 err = %v", err)
	}
	if len(page2.Items) != 1 || page2.Items[0].JobID != "j1" {
		t.Fatalf("page2 = %+v, want exactly [j1]", page2.Items)
	}
	if page2.NextCursor != "" {
		t.Errorf("expected empty next cursor on final page, got %q", page2.NextCursor)
	}
}

func TestApplyQueueRepo_List_InvalidCursor_RestartsFromFirstPage(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "not-a-valid-cursor", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1 (corrupt cursor should restart at page 1)", len(result.Items))
	}
}

func TestApplyQueueRepo_List_LimitClamping(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	now := time.Now()
	for i := 0; i < 5; i++ {
		id := "j" + string(rune('0'+i))
		seedJob(t, db, id, "c1", "Job "+id)
		savedAt := now.Add(time.Duration(i) * time.Second)
		seedUserJobState(t, db, testUserID, id, nil, &savedAt, now.Add(time.Duration(i)*time.Second))
	}
	repo := applyqueuesvc.NewRepositoryService(db)

	// limit <= 0 defaults to 20.
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 0)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 5 {
		t.Fatalf("len(items) = %d, want 5 (default limit should cover all 5 rows)", len(result.Items))
	}

	// limit > 100 clamps to 100 (still returns all 5 since that's every row).
	result2, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 500)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result2.Items) != 5 {
		t.Fatalf("len(items) = %d, want 5", len(result2.Items))
	}
}

func TestApplyQueueRepo_SetResume_Attach_CreatesRowAndReturnsView(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedResume(t, db, "r1", testUserID, "My Resume")

	repo := applyqueuesvc.NewRepositoryService(db)
	resumeID := "r1"
	view, err := repo.SetResume(context.Background(), testUserID, "j1", &resumeID)
	if err != nil {
		t.Fatalf("SetResume err = %v", err)
	}
	if view == nil {
		t.Fatal("expected non-nil view on attach")
	}
	if view.JobID != "j1" || view.ResumeID == nil || *view.ResumeID != "r1" {
		t.Errorf("view = %+v, want job_id=j1 resume_id=r1", view)
	}
	if view.ResumeLabel == nil || *view.ResumeLabel != "My Resume" {
		t.Errorf("ResumeLabel = %v, want %q", view.ResumeLabel, "My Resume")
	}
}

func TestApplyQueueRepo_SetResume_Attach_ReplacesExisting(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedResume(t, db, "r1", testUserID, "Resume One")
	seedResume(t, db, "r2", testUserID, "Resume Two")

	repo := applyqueuesvc.NewRepositoryService(db)
	r1 := "r1"
	if _, err := repo.SetResume(context.Background(), testUserID, "j1", &r1); err != nil {
		t.Fatalf("first attach err = %v", err)
	}
	r2 := "r2"
	view, err := repo.SetResume(context.Background(), testUserID, "j1", &r2)
	if err != nil {
		t.Fatalf("second attach err = %v", err)
	}
	if view == nil || view.ResumeID == nil || *view.ResumeID != "r2" {
		t.Fatalf("view = %+v, want resume_id=r2 (replace, not duplicate)", view)
	}

	// Only one row should exist for (user, job) — no duplicate PK violation
	// and no lingering second row.
	var count int64
	if err := db.Table("resume_job_attachments").
		Where("user_id = ? AND job_id = ?", testUserID, "j1").
		Count(&count).Error; err != nil {
		t.Fatalf("count err = %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1", count)
	}
}

func TestApplyQueueRepo_SetResume_Clear_DeletesRow(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedResume(t, db, "r1", testUserID, "My Resume")
	repo := applyqueuesvc.NewRepositoryService(db)
	r1 := "r1"
	if _, err := repo.SetResume(context.Background(), testUserID, "j1", &r1); err != nil {
		t.Fatalf("attach err = %v", err)
	}

	view, err := repo.SetResume(context.Background(), testUserID, "j1", nil)
	if err != nil {
		t.Fatalf("clear err = %v", err)
	}
	if view != nil {
		t.Fatalf("expected nil view on clear, got %+v", view)
	}

	var count int64
	if err := db.Table("resume_job_attachments").
		Where("user_id = ? AND job_id = ?", testUserID, "j1").
		Count(&count).Error; err != nil {
		t.Fatalf("count err = %v", err)
	}
	if count != 0 {
		t.Fatalf("row count = %d, want 0 after clear", count)
	}
}

func TestApplyQueueRepo_SetResume_Clear_NoExistingRow_Idempotent(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	repo := applyqueuesvc.NewRepositoryService(db)

	view, err := repo.SetResume(context.Background(), testUserID, "j1", nil)
	if err != nil {
		t.Fatalf("clear-with-no-row err = %v", err)
	}
	if view != nil {
		t.Fatalf("expected nil view, got %+v", view)
	}
}

func TestApplyQueueRepo_HasEngagement(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)

	checker := applyqueuesvc.NewEngagementChecker(db)

	has, err := checker.HasEngagement(context.Background(), testUserID, "j1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !has {
		t.Error("HasEngagement = false, want true")
	}

	has2, err := checker.HasEngagement(context.Background(), testUserID, "no-such-job")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if has2 {
		t.Error("HasEngagement = true, want false for unengaged job")
	}
}

func TestApplyQueueRepo_ResumeOwnedByUser(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedResume(t, db, "r1", testUserID, "My Resume")

	checker := applyqueuesvc.NewResumeOwnershipChecker(db)

	owned, err := checker.ResumeOwnedByUser(context.Background(), testUserID, "r1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !owned {
		t.Error("ResumeOwnedByUser = false, want true")
	}

	ownedByOther, err := checker.ResumeOwnedByUser(context.Background(), "someone-else", "r1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ownedByOther {
		t.Error("ResumeOwnedByUser = true for a different user, want false")
	}

	ownedMissing, err := checker.ResumeOwnedByUser(context.Background(), testUserID, "no-such-resume")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ownedMissing {
		t.Error("ResumeOwnedByUser = true for a nonexistent resume, want false")
	}
}

// ─── Job keywords (replaces resume recommendation, issue #264 follow-up) ────

func TestApplyQueueRepo_List_JobKeywords_PopulatedFromTechTags(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	seedTechTags(t, db, "j1", "c1", "Go", "PostgreSQL")
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(result.Items))
	}
	got := result.Items[0].JobKeywords
	if len(got) != 2 || got[0] != "Go" || got[1] != "PostgreSQL" {
		t.Errorf("JobKeywords = %v, want [Go PostgreSQL]", got)
	}
}

func TestApplyQueueRepo_List_JobKeywords_NoTechTagsIsEmpty(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer") // no seedTechTags call
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items[0].JobKeywords) != 0 {
		t.Errorf("expected empty JobKeywords for a job with no tech tags, got %v", result.Items[0].JobKeywords)
	}
}

func TestApplyQueueRepo_List_JobKeywords_IndependentPerJobOnSamePage(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Backend Engineer")
	seedJob(t, db, "j2", "c1", "Frontend Engineer")
	seedTechTags(t, db, "j1", "c1", "Go", "PostgreSQL")
	seedTechTags(t, db, "j2", "c1", "React", "TypeScript")
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)
	seedUserJobState(t, db, testUserID, "j2", nil, &savedAt, now.Add(time.Second))

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(result.Items))
	}

	byJobID := map[string][]string{}
	for _, item := range result.Items {
		byJobID[item.JobID] = item.JobKeywords
	}
	if len(byJobID["j1"]) != 2 || byJobID["j1"][0] != "Go" {
		t.Errorf("j1 JobKeywords = %v, want [Go PostgreSQL]", byJobID["j1"])
	}
	if len(byJobID["j2"]) != 2 || byJobID["j2"][0] != "React" {
		t.Errorf("j2 JobKeywords = %v, want [React TypeScript]", byJobID["j2"])
	}
}
