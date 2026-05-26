package services_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	"gorm.io/gorm"
)

// seedJob inserts a minimal job row.
func seedJob(t *testing.T, db *gorm.DB, id, companyID, title, jobPostID string, added time.Time) {
	t.Helper()
	row := jobmodel.Job{
		ID:        id,
		JobPostID: jobPostID,
		CompanyID: companyID,
		Title:     title,
		URL:       "https://example.com/" + id,
		AddedAt:   added,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed job %s: %v", id, err)
	}
}

// closedJobDB returns a DB whose underlying *sql.DB has been closed.
func closedJobDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newJobTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

// TestJobStore_GetJobByID_HappyPath inserts a row through GORM and reads it
// back via the store. Locks the Company JOIN works on sqlite (FK exists).
func TestJobStore_GetJobByID_HappyPath(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer", "post-1", time.Now())

	store := jobsvc.NewJobStore(db)
	job, err := store.GetJobByID(context.Background(), "j1")
	if err != nil {
		t.Fatalf("GetJobByID err = %v", err)
	}
	if job.ID != "j1" {
		t.Errorf("id = %q, want j1", job.ID)
	}
	if job.Company.Name != "Acme" {
		t.Errorf("company name = %q, want Acme (preloaded via join)", job.Company.Name)
	}
}

// TestJobStore_GetJobByID_NotFound asserts a missing ID surfaces as a
// gorm.ErrRecordNotFound (the impl wraps it as NOT_FOUND).
func TestJobStore_GetJobByID_NotFound(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	store := jobsvc.NewJobStore(db)
	_, err := store.GetJobByID(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing id")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("err = %v, want gorm.ErrRecordNotFound", err)
	}
}

// TestJobStore_GetJobsByIDs_EmptyReturnsNil — passing an empty slice short-
// circuits without a query.
func TestJobStore_GetJobsByIDs_EmptyReturnsNil(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	store := jobsvc.NewJobStore(db)
	jobs, err := store.GetJobsByIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if jobs != nil {
		t.Errorf("jobs = %+v, want nil for empty IDs", jobs)
	}
}

// TestJobStore_GetJobsByIDs_FetchesMultiple verifies the WHERE id IN ? query.
func TestJobStore_GetJobsByIDs_FetchesMultiple(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	now := time.Now()
	seedJob(t, db, "j1", "c1", "T1", "p1", now)
	seedJob(t, db, "j2", "c1", "T2", "p2", now)
	seedJob(t, db, "j3", "c1", "T3", "p3", now)

	store := jobsvc.NewJobStore(db)
	jobs, err := store.GetJobsByIDs(context.Background(), []string{"j1", "j3"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("len = %d, want 2", len(jobs))
	}
	got := map[string]bool{}
	for _, j := range jobs {
		got[j.ID] = true
	}
	if !got["j1"] || !got["j3"] {
		t.Errorf("jobs = %v, want j1+j3", got)
	}
	if got["j2"] {
		t.Errorf("j2 leaked into result: %v", got)
	}
}

// TestJobStore_GetJobPostIDs_EmptyShortCircuit — empty input returns nil.
func TestJobStore_GetJobPostIDs_EmptyShortCircuit(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	store := jobsvc.NewJobStore(db)
	out, err := store.GetJobPostIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out != nil {
		t.Errorf("out = %v, want nil", out)
	}
}

// TestJobStore_GetJobPostIDs_PlucksPostIDs — verifies the Pluck call returns
// only the job_post_id column.
func TestJobStore_GetJobPostIDs_PlucksPostIDs(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	now := time.Now()
	seedJob(t, db, "j1", "c1", "T1", "post-1", now)
	seedJob(t, db, "j2", "c1", "T2", "post-2", now)

	store := jobsvc.NewJobStore(db)
	out, err := store.GetJobPostIDs(context.Background(), []string{"j1", "j2"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(out) != 2 {
		t.Errorf("len = %d, want 2", len(out))
	}
	got := map[string]bool{}
	for _, p := range out {
		got[p] = true
	}
	if !got["post-1"] || !got["post-2"] {
		t.Errorf("post ids = %v, want post-1 + post-2", got)
	}
}

// TestJobStore_CountAll counts all rows; new rows bump the count.
func TestJobStore_CountAll(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	store := jobsvc.NewJobStore(db)
	n, err := store.CountAll(context.Background())
	if err != nil {
		t.Fatalf("CountAll err = %v", err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0", n)
	}
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T1", "p1", time.Now())
	seedJob(t, db, "j2", "c1", "T2", "p2", time.Now())
	n, err = store.CountAll(context.Background())
	if err != nil {
		t.Fatalf("CountAll err = %v", err)
	}
	if n != 2 {
		t.Errorf("count = %d, want 2", n)
	}
}

// TestJobStore_CountAll_DBError — closing the underlying *sql.DB surfaces an
// error rather than returning zero count silently.
func TestJobStore_CountAll_DBError(t *testing.T) {
	t.Parallel()
	store := jobsvc.NewJobStore(closedJobDB(t))
	_, err := store.CountAll(context.Background())
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

// TestJobStore_InsertJob_HappyPath asserts a fresh insert reads back via the
// same store.
func TestJobStore_InsertJob_HappyPath(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	store := jobsvc.NewJobStore(db)

	row := &jobmodel.Job{
		ID:        "j1",
		JobPostID: "post-1",
		CompanyID: "c1",
		Title:     "Engineer",
		URL:       "https://example.com/j1",
		AddedAt:   time.Now(),
	}
	if err := store.InsertJob(context.Background(), row); err != nil {
		t.Fatalf("InsertJob err = %v", err)
	}
	got, err := store.GetJobByID(context.Background(), "j1")
	if err != nil {
		t.Fatalf("readback err = %v", err)
	}
	if got.ID != "j1" || got.Title != "Engineer" {
		t.Errorf("readback = %+v", got)
	}
}

// TestJobStore_InsertJob_DuplicateJobPostID_ReturnsErrJobAlreadyExists — a
// replay with the same job_post_id (even a different id) is idempotent: the
// second insert is a no-op via ON CONFLICT DO NOTHING and returns the typed
// sentinel so the enricher can skip dependent writes. Exactly one row remains.
func TestJobStore_InsertJob_DuplicateJobPostID_ReturnsErrJobAlreadyExists(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	store := jobsvc.NewJobStore(db)
	first := &jobmodel.Job{
		ID: "j-dup-1", JobPostID: "post", CompanyID: "c1", Title: "T1", URL: "u1", AddedAt: time.Now(),
	}
	if err := store.InsertJob(context.Background(), first); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	// Different primary key, same job_post_id — the conflict target.
	second := &jobmodel.Job{
		ID: "j-dup-2", JobPostID: "post", CompanyID: "c1", Title: "T2", URL: "u2", AddedAt: time.Now(),
	}
	err := store.InsertJob(context.Background(), second)
	if !errors.Is(err, apperr.ErrJobAlreadyExists) {
		t.Fatalf("want ErrJobAlreadyExists, got %v", err)
	}
	var count int64
	if err := db.Model(&jobmodel.Job{}).Where("job_post_id = ?", "post").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("row count for job_post_id=post = %d, want 1", count)
	}
}

// TestJobStore_DeleteJob_RemovesRow — verifies the delete and confirms no
// row remains via GetJobByID returning NotFound.
func TestJobStore_DeleteJob_RemovesRow(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p1", time.Now())
	store := jobsvc.NewJobStore(db)
	if err := store.DeleteJob(context.Background(), "j1"); err != nil {
		t.Fatalf("DeleteJob err = %v", err)
	}
	_, err := store.GetJobByID(context.Background(), "j1")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("after delete, err = %v, want gorm.ErrRecordNotFound", err)
	}
}

// TestJobStore_DeleteJob_NonExistent_NoError — deleting a missing row is a
// no-op (GORM Delete with WHERE returns nil affected rows but no error).
func TestJobStore_DeleteJob_NonExistent_NoError(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	store := jobsvc.NewJobStore(db)
	if err := store.DeleteJob(context.Background(), "ghost"); err != nil {
		t.Errorf("delete missing id should not error, got %v", err)
	}
}

// TestJobStore_GetDistinctCompanyNames returns sorted distinct names from
// the companies table for jobs present.
func TestJobStore_GetDistinctCompanyNames(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Zeta")
	seedCompany(t, db, "c2", "Acme")
	seedCompany(t, db, "c3", "Globex")
	now := time.Now()
	// Only c1 + c2 have jobs; c3 must be excluded.
	seedJob(t, db, "j1", "c1", "T1", "p1", now)
	seedJob(t, db, "j2", "c2", "T2", "p2", now)
	seedJob(t, db, "j3", "c1", "T3", "p3", now) // duplicate company → still one entry

	store := jobsvc.NewJobStore(db)
	names, err := store.GetDistinctCompanyNames(context.Background())
	if err != nil {
		t.Fatalf("GetDistinctCompanyNames err = %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("len = %d, want 2 (Acme + Zeta), got %v", len(names), names)
	}
	// Sorted ascending
	if names[0] != "Acme" || names[1] != "Zeta" {
		t.Errorf("names = %v, want [Acme Zeta]", names)
	}
}

// TestJobStore_GetDistinctCompanyNames_Empty returns an empty slice (not
// nil-typed error) when the jobs table has no rows.
func TestJobStore_GetDistinctCompanyNames_Empty(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	store := jobsvc.NewJobStore(db)
	names, err := store.GetDistinctCompanyNames(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(names) != 0 {
		t.Errorf("names = %v, want empty", names)
	}
}

// TestJobStore_GetJobs_HappyPath — basic limit-bounded list ordered by
// added_at DESC.
func TestJobStore_GetJobs_HappyPath(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	t0 := time.Now()
	seedJob(t, db, "old", "c1", "Old", "p1", t0.Add(-2*time.Hour))
	seedJob(t, db, "mid", "c1", "Mid", "p2", t0.Add(-time.Hour))
	seedJob(t, db, "new", "c1", "New", "p3", t0)

	store := jobsvc.NewJobStore(db)
	jobs, next, err := store.GetJobs(context.Background(), dummyFilter(), "", 10)
	if err != nil {
		t.Fatalf("GetJobs err = %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}
	// Default sort = added_at DESC → newest first.
	if jobs[0].ID != "new" || jobs[1].ID != "mid" || jobs[2].ID != "old" {
		t.Errorf("order = [%s,%s,%s], want [new,mid,old]", jobs[0].ID, jobs[1].ID, jobs[2].ID)
	}
	if next != "" {
		t.Errorf("next cursor = %q, want empty (only one page)", next)
	}
}

// TestJobStore_GetJobs_LimitClampedToDefault — limit<=0 clamps to 20.
// We assert by inserting >20 rows and confirming exactly 20 are returned with
// a non-empty cursor.
func TestJobStore_GetJobs_LimitClampedToDefault(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	t0 := time.Now()
	for i := 0; i < 25; i++ {
		seedJob(t, db, jobID(i), "c1", "T", "p"+jobID(i), t0.Add(time.Duration(i)*time.Second))
	}
	store := jobsvc.NewJobStore(db)
	jobs, next, err := store.GetJobs(context.Background(), dummyFilter(), "", 0)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(jobs) != 20 {
		t.Errorf("len = %d, want 20 (default cap)", len(jobs))
	}
	if next == "" {
		t.Error("next cursor empty; want non-empty (more pages available)")
	}
}

// TestJobStore_GetJobs_LimitClampedTo100 — limit>100 caps at 100.
func TestJobStore_GetJobs_LimitClampedTo100(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	t0 := time.Now()
	for i := 0; i < 110; i++ {
		seedJob(t, db, jobID(i), "c1", "T", "p"+jobID(i), t0.Add(time.Duration(i)*time.Second))
	}
	store := jobsvc.NewJobStore(db)
	jobs, _, err := store.GetJobs(context.Background(), dummyFilter(), "", 200)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(jobs) != 100 {
		t.Errorf("len = %d, want 100 (cap)", len(jobs))
	}
}

// TestJobStore_GetJobs_CursorPagination — two successive pages cover all
// rows without overlap. Crucial regression check for keyset pagination.
func TestJobStore_GetJobs_CursorPagination(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	t0 := time.Now()
	for i := 0; i < 10; i++ {
		// Stagger added_at so the keyset cursor is deterministic.
		seedJob(t, db, jobID(i), "c1", "T", "p"+jobID(i), t0.Add(time.Duration(i)*time.Minute))
	}
	store := jobsvc.NewJobStore(db)

	page1, next, err := store.GetJobs(context.Background(), dummyFilter(), "", 4)
	if err != nil {
		t.Fatalf("page1 err = %v", err)
	}
	if len(page1) != 4 {
		t.Fatalf("page1 len = %d, want 4", len(page1))
	}
	if next == "" {
		t.Fatal("page1 next cursor empty; want non-empty")
	}

	page2, next2, err := store.GetJobs(context.Background(), dummyFilter(), next, 4)
	if err != nil {
		t.Fatalf("page2 err = %v", err)
	}
	if len(page2) != 4 {
		t.Fatalf("page2 len = %d, want 4", len(page2))
	}

	seen := map[string]bool{}
	for _, j := range page1 {
		seen[j.ID] = true
	}
	for _, j := range page2 {
		if seen[j.ID] {
			t.Errorf("job %s appears on both pages — cursor overlap bug", j.ID)
		}
		seen[j.ID] = true
	}

	page3, _, err := store.GetJobs(context.Background(), dummyFilter(), next2, 4)
	if err != nil {
		t.Fatalf("page3 err = %v", err)
	}
	if len(page3) != 2 {
		t.Errorf("page3 len = %d, want 2 (last partial page)", len(page3))
	}
}

// TestJobStore_GetJobs_BadCursor_TreatedAsFirstPage — a garbage cursor must
// return the first page (per decodeCursor contract).
func TestJobStore_GetJobs_BadCursor_TreatedAsFirstPage(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	t0 := time.Now()
	seedJob(t, db, "j1", "c1", "T", "p1", t0)
	seedJob(t, db, "j2", "c1", "T", "p2", t0)
	store := jobsvc.NewJobStore(db)

	jobs, _, err := store.GetJobs(context.Background(), dummyFilter(), "!!garbage-cursor!!", 10)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("len = %d, want 2 (bad cursor → first page)", len(jobs))
	}
}

// TestJobStore_GetJobs_DBError — passing a closed DB surfaces the underlying
// driver error.
func TestJobStore_GetJobs_DBError(t *testing.T) {
	t.Parallel()
	store := jobsvc.NewJobStore(closedJobDB(t))
	_, _, err := store.GetJobs(context.Background(), dummyFilter(), "", 10)
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
}

// TestJobStore_GetJobByID_DBError — closed DB surfaces error rather than nil.
func TestJobStore_GetJobByID_DBError(t *testing.T) {
	t.Parallel()
	store := jobsvc.NewJobStore(closedJobDB(t))
	_, err := store.GetJobByID(context.Background(), "any")
	if err == nil || errors.Is(err, sql.ErrConnDone) == false && err.Error() == "" {
		// Just guarantee a non-nil error is returned.
		if err == nil {
			t.Fatal("expected error from closed DB")
		}
	}
}

// -------- helpers --------

func dummyFilter() jobdto.JobFilter {
	return jobdto.JobFilter{}
}

// jobID generates a stable test id padded to two digits so lexical ID sort
// matches insert order — important because tiebreaker is id DESC.
func jobID(i int) string {
	const prefix = "j-"
	if i < 10 {
		return prefix + "0" + string(rune('0'+i))
	}
	return prefix + string(rune('0'+i/10)) + string(rune('0'+i%10))
}

// Compile-time sanity check that the typed sentinel still imports.
var _ = apperr.ErrInternal
