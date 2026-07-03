package services_test

import (
	"context"
	"testing"
	"time"

	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// upsertSaved is a helper that inserts a user_job_state row for (uid, jid)
// with status=saved. Tests use it to populate the per-user feed.
func upsertSaved(t *testing.T, repo jobsvc.UserJobStateRepository, uid, jid string) {
	t.Helper()
	if err := repo.Upsert(context.Background(), uid, jid, jobdto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("upsert (%s,%s): %v", uid, jid, err)
	}
}

// TestUserJobStateRepo_GetJobsByStatus_HappyPath — returns only rows whose
// user_job_state matches both the user and status filter.
func TestUserJobStateRepo_GetJobsByStatus_HappyPath(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	t0 := time.Now()
	seedJob(t, db, "j1", "c1", "T1", "p1", t0.Add(2*time.Minute))
	seedJob(t, db, "j2", "c1", "T2", "p2", t0.Add(time.Minute))
	seedJob(t, db, "j3", "c1", "T3", "p3", t0)
	repo := jobsvc.NewUserJobStateRepository(db)
	upsertSaved(t, repo, "u1", "j1")
	upsertSaved(t, repo, "u1", "j2")
	// j3 has no row for u1 → excluded.
	// j2 belongs to u2 — must not leak.
	if err := repo.Upsert(context.Background(), "u2", "j3", jobdto.UpsertState{Status: jobmodel.StatusSaved}); err != nil {
		t.Fatalf("seed u2: %v", err)
	}

	jobs, _, err := repo.GetJobsByStatus(context.Background(), "u1",
		jobdto.JobFilter{Status: jobmodel.StatusSaved}, "", 10)
	if err != nil {
		t.Fatalf("GetJobsByStatus err = %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len = %d, want 2", len(jobs))
	}
	// Default sort: added_at DESC → j1, j2.
	if jobs[0].ID != "j1" || jobs[1].ID != "j2" {
		t.Errorf("order = [%s,%s], want [j1,j2]", jobs[0].ID, jobs[1].ID)
	}
}

// TestUserJobStateRepo_GetJobsByStatus_Pagination — second page picks up
// where first left off.
func TestUserJobStateRepo_GetJobsByStatus_Pagination(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	t0 := time.Now()
	for i := 0; i < 4; i++ {
		jid := jobID(i)
		seedJob(t, db, jid, "c1", "T", "p"+jid, t0.Add(time.Duration(i)*time.Minute))
	}
	repo := jobsvc.NewUserJobStateRepository(db)
	for i := 0; i < 4; i++ {
		upsertSaved(t, repo, "u1", jobID(i))
	}

	page1, cur, err := repo.GetJobsByStatus(context.Background(), "u1",
		jobdto.JobFilter{Status: jobmodel.StatusSaved}, "", 2)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}
	if cur == "" {
		t.Fatal("page1 next cursor empty")
	}
	page2, _, err := repo.GetJobsByStatus(context.Background(), "u1",
		jobdto.JobFilter{Status: jobmodel.StatusSaved}, cur, 2)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("page2 len = %d, want 2", len(page2))
	}
	seen := map[string]bool{}
	for _, j := range page1 {
		seen[j.ID] = true
	}
	for _, j := range page2 {
		if seen[j.ID] {
			t.Errorf("duplicate %s across pages", j.ID)
		}
	}
}

// TestUserJobStateRepo_GetJobsByStatus_BadCursor — garbage cursor falls back
// to first page silently.
func TestUserJobStateRepo_GetJobsByStatus_BadCursor(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "T", "p", time.Now())
	repo := jobsvc.NewUserJobStateRepository(db)
	upsertSaved(t, repo, "u1", "j1")

	jobs, _, err := repo.GetJobsByStatus(context.Background(), "u1",
		jobdto.JobFilter{Status: jobmodel.StatusSaved}, "!!garbage!!", 10)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("len = %d, want 1 (bad cursor → first page)", len(jobs))
	}
}

// TestUserJobStateRepo_GetJobsByStatus_CompaniesFilter exercises the
// companies WHERE branch on the per-user feed.
func TestUserJobStateRepo_GetJobsByStatus_CompaniesFilter(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedCompany(t, db, "c2", "Globex")
	t0 := time.Now()
	seedJob(t, db, "j1", "c1", "T1", "p1", t0)
	seedJob(t, db, "j2", "c2", "T2", "p2", t0)
	repo := jobsvc.NewUserJobStateRepository(db)
	upsertSaved(t, repo, "u1", "j1")
	upsertSaved(t, repo, "u1", "j2")

	jobs, _, err := repo.GetJobsByStatus(context.Background(), "u1",
		jobdto.JobFilter{Status: jobmodel.StatusSaved, Companies: []string{"Acme"}}, "", 10)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "j1" {
		t.Errorf("got %v, want only j1", idSlice(jobs))
	}
}

// TestUserJobStateRepo_GetJobsByStatus_RemoteFilter — Remote pointer / on-
// site bool / sponsorship pointer / requires_citizen all share the same
// boolean filter pattern; covering one is enough to exercise the branch.
func TestUserJobStateRepo_GetJobsByStatus_RemoteFilter(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	t0 := time.Now()
	rem := jobmodel.Job{ID: "j-r", JobPostID: "p", CompanyID: "c1", Title: "T", URL: "u", AddedAt: t0, Remote: true}
	on := jobmodel.Job{ID: "j-o", JobPostID: "p2", CompanyID: "c1", Title: "T", URL: "u", AddedAt: t0, Remote: false}
	if err := db.Create(&rem).Error; err != nil {
		t.Fatalf("seed rem: %v", err)
	}
	if err := db.Create(&on).Error; err != nil {
		t.Fatalf("seed on: %v", err)
	}
	repo := jobsvc.NewUserJobStateRepository(db)
	upsertSaved(t, repo, "u1", "j-r")
	upsertSaved(t, repo, "u1", "j-o")

	yes := true
	jobs, _, err := repo.GetJobsByStatus(context.Background(), "u1",
		jobdto.JobFilter{Status: jobmodel.StatusSaved, Remote: &yes}, "", 10)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "j-r" {
		t.Errorf("got %v, want only j-r", idSlice(jobs))
	}

	// OnSite → remote=false
	jobs2, _, err := repo.GetJobsByStatus(context.Background(), "u1",
		jobdto.JobFilter{Status: jobmodel.StatusSaved, OnSite: true}, "", 10)
	if err != nil {
		t.Fatalf("on_site err = %v", err)
	}
	if len(jobs2) != 1 || jobs2[0].ID != "j-o" {
		t.Errorf("on_site got %v, want only j-o", idSlice(jobs2))
	}
}

// TestUserJobStateRepo_GetJobsByStatus_EmployTypeAndRequirements covers
// employ_types IN, sponsorship pointer, NoRequirements bool, and
// requires_citizen pointer branches in GetJobsByStatus.
func TestUserJobStateRepo_GetJobsByStatus_EmployTypeAndRequirements(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	t0 := time.Now()
	ft := "full_time"
	pt := "part_time"
	a := jobmodel.Job{ID: "j-a", JobPostID: "p", CompanyID: "c1", Title: "T", URL: "u", AddedAt: t0, EmployType: &ft, Sponsorship: true, RequiresCitizen: false, SecurityClearance: false}
	b := jobmodel.Job{ID: "j-b", JobPostID: "p2", CompanyID: "c1", Title: "T", URL: "u", AddedAt: t0, EmployType: &pt, Sponsorship: false, RequiresCitizen: true}
	for _, row := range []jobmodel.Job{a, b} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	repo := jobsvc.NewUserJobStateRepository(db)
	upsertSaved(t, repo, "u1", "j-a")
	upsertSaved(t, repo, "u1", "j-b")

	// employ_types = full_time → only j-a
	jobs, _, err := repo.GetJobsByStatus(context.Background(), "u1",
		jobdto.JobFilter{Status: jobmodel.StatusSaved, EmployTypes: []string{"full_time"}}, "", 10)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "j-a" {
		t.Errorf("employ_types got %v, want only j-a", idSlice(jobs))
	}

	// sponsorship=true → only j-a
	yes := true
	jobs2, _, err := repo.GetJobsByStatus(context.Background(), "u1",
		jobdto.JobFilter{Status: jobmodel.StatusSaved, Sponsorship: &yes}, "", 10)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(jobs2) != 1 || jobs2[0].ID != "j-a" {
		t.Errorf("sponsorship got %v, want only j-a", idSlice(jobs2))
	}

	// NoRequirements=true → only j-a (no security_clearance, no citizen)
	jobs3, _, err := repo.GetJobsByStatus(context.Background(), "u1",
		jobdto.JobFilter{Status: jobmodel.StatusSaved, NoRequirements: true}, "", 10)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(jobs3) != 1 || jobs3[0].ID != "j-a" {
		t.Errorf("no_req got %v, want only j-a", idSlice(jobs3))
	}

	// requires_citizen=true → only j-b
	jobs4, _, err := repo.GetJobsByStatus(context.Background(), "u1",
		jobdto.JobFilter{Status: jobmodel.StatusSaved, RequiresCitizen: &yes}, "", 10)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(jobs4) != 1 || jobs4[0].ID != "j-b" {
		t.Errorf("rc=true got %v, want only j-b", idSlice(jobs4))
	}
}

// TestUserJobStateRepo_GetJobsByStatus_ExperienceRange — closed-range
// branch on the per-user feed.
func TestUserJobStateRepo_GetJobsByStatus_ExperienceRange(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	t0 := time.Now()
	y3 := 3
	y7 := 7
	seedJobWithExp(t, db, "j-3", "c1", "T", &y3, t0)
	seedJobWithExp(t, db, "j-7", "c1", "T", &y7, t0)
	repo := jobsvc.NewUserJobStateRepository(db)
	upsertSaved(t, repo, "u1", "j-3")
	upsertSaved(t, repo, "u1", "j-7")

	ten := 10
	jobs, _, err := repo.GetJobsByStatus(context.Background(), "u1", jobdto.JobFilter{
		Status:           jobmodel.StatusSaved,
		ExperienceRanges: []jobdto.ExperienceRange{{Min: 5, Max: &ten}},
	}, "", 10)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "j-7" {
		t.Errorf("got %v, want only j-7", idSlice(jobs))
	}
}

// TestUserJobStateRepo_GetJobsByStatus_ExperienceRange_OpenAndMin0 covers
// the Max==nil and Min==0 sub-branches.
func TestUserJobStateRepo_GetJobsByStatus_ExperienceRange_OpenAndMin0(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	t0 := time.Now()
	y3 := 3
	y7 := 7
	seedJobWithExp(t, db, "j-3", "c1", "T", &y3, t0)
	seedJobWithExp(t, db, "j-7", "c1", "T", &y7, t0)
	seedJobWithExp(t, db, "j-null", "c1", "T", nil, t0)
	repo := jobsvc.NewUserJobStateRepository(db)
	upsertSaved(t, repo, "u1", "j-3")
	upsertSaved(t, repo, "u1", "j-7")
	upsertSaved(t, repo, "u1", "j-null")

	// Open-ended Min=5
	jobs, _, err := repo.GetJobsByStatus(context.Background(), "u1", jobdto.JobFilter{
		Status:           jobmodel.StatusSaved,
		ExperienceRanges: []jobdto.ExperienceRange{{Min: 5, Max: nil}},
	}, "", 10)
	if err != nil {
		t.Fatalf("open err = %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "j-7" {
		t.Errorf("open got %v, want only j-7", idSlice(jobs))
	}

	// Min=0, Max=5 → includes 3 and NULL
	five := 5
	jobs2, _, err := repo.GetJobsByStatus(context.Background(), "u1", jobdto.JobFilter{
		Status:           jobmodel.StatusSaved,
		ExperienceRanges: []jobdto.ExperienceRange{{Min: 0, Max: &five}},
	}, "", 10)
	if err != nil {
		t.Fatalf("min0 err = %v", err)
	}
	got := idSlice(jobs2)
	if len(got) != 2 {
		t.Errorf("min0 got %v, want 2 (j-3 + j-null)", got)
	}
}

// TestUserJobStateRepo_GetJobsByStatus_CombinedStatuses — filter.Statuses
// (the combined multi-status view used by e.g. the Tracking tab) returns the
// union of jobs across every listed status, scoped to the current user, and
// excludes statuses not in the list.
func TestUserJobStateRepo_GetJobsByStatus_CombinedStatuses(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	t0 := time.Now()
	seedJob(t, db, "j-applied", "c1", "T1", "p1", t0.Add(3*time.Minute))
	seedJob(t, db, "j-interviewing", "c1", "T2", "p2", t0.Add(2*time.Minute))
	seedJob(t, db, "j-offer", "c1", "T3", "p3", t0.Add(time.Minute))
	seedJob(t, db, "j-rejected", "c1", "T4", "p4", t0)
	repo := jobsvc.NewUserJobStateRepository(db)
	ctx := context.Background()
	if err := repo.Upsert(ctx, "u1", "j-applied", jobdto.UpsertState{Status: jobmodel.StatusApplied}); err != nil {
		t.Fatalf("seed applied: %v", err)
	}
	if err := repo.Upsert(ctx, "u1", "j-interviewing", jobdto.UpsertState{Status: jobmodel.StatusInterviewing}); err != nil {
		t.Fatalf("seed interviewing: %v", err)
	}
	if err := repo.Upsert(ctx, "u1", "j-offer", jobdto.UpsertState{Status: jobmodel.StatusOffer}); err != nil {
		t.Fatalf("seed offer: %v", err)
	}
	if err := repo.Upsert(ctx, "u1", "j-rejected", jobdto.UpsertState{Status: jobmodel.StatusRejected}); err != nil {
		t.Fatalf("seed rejected: %v", err)
	}

	jobs, _, err := repo.GetJobsByStatus(ctx, "u1",
		jobdto.JobFilter{Statuses: []string{jobmodel.StatusApplied, jobmodel.StatusInterviewing}}, "", 10)
	if err != nil {
		t.Fatalf("GetJobsByStatus err = %v", err)
	}
	got := idSlice(jobs)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %v", len(got), got)
	}
	want := map[string]bool{"j-applied": true, "j-interviewing": true}
	for _, id := range got {
		if !want[id] {
			t.Errorf("unexpected job %s in combined-status result", id)
		}
	}
}

// TestUserJobStateRepo_GetJobsByStatus_StatusesTakesPrecedenceOverStatus —
// when both Status and Statuses are set, Statuses wins.
func TestUserJobStateRepo_GetJobsByStatus_StatusesTakesPrecedenceOverStatus(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	t0 := time.Now()
	seedJob(t, db, "j-applied", "c1", "T1", "p1", t0.Add(time.Minute))
	seedJob(t, db, "j-offer", "c1", "T2", "p2", t0)
	repo := jobsvc.NewUserJobStateRepository(db)
	ctx := context.Background()
	if err := repo.Upsert(ctx, "u1", "j-applied", jobdto.UpsertState{Status: jobmodel.StatusApplied}); err != nil {
		t.Fatalf("seed applied: %v", err)
	}
	if err := repo.Upsert(ctx, "u1", "j-offer", jobdto.UpsertState{Status: jobmodel.StatusOffer}); err != nil {
		t.Fatalf("seed offer: %v", err)
	}

	jobs, _, err := repo.GetJobsByStatus(ctx, "u1",
		jobdto.JobFilter{Status: jobmodel.StatusOffer, Statuses: []string{jobmodel.StatusApplied}}, "", 10)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "j-applied" {
		t.Errorf("got %v, want only j-applied (Statuses must take precedence over Status)", idSlice(jobs))
	}
}

// TestUserJobStateRepo_GetJobsByStatus_PaginatesWithSortByCompany exercises
// the company-sort branch through the per-user repo.
func TestUserJobStateRepo_GetJobsByStatus_PaginatesWithSortByCompany(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedCompany(t, db, "c2", "Globex")
	t0 := time.Now()
	seedJob(t, db, "j1", "c1", "T1", "p1", t0)
	seedJob(t, db, "j2", "c2", "T2", "p2", t0)
	repo := jobsvc.NewUserJobStateRepository(db)
	upsertSaved(t, repo, "u1", "j1")
	upsertSaved(t, repo, "u1", "j2")

	// Ascending by company name → Acme, Globex.
	jobs, _, err := repo.GetJobsByStatus(context.Background(), "u1", jobdto.JobFilter{
		Status: jobmodel.StatusSaved, SortBy: "company", SortDir: "asc",
	}, "", 10)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(jobs) != 2 || jobs[0].ID != "j1" || jobs[1].ID != "j2" {
		t.Errorf("got %v, want [j1,j2]", idSlice(jobs))
	}
}
