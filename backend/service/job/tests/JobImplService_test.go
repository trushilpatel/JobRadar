package services_test

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	"github.com/trushilpatel/jobradar/platform/cache"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	"gorm.io/gorm"
)

// --- Test doubles ---

type fakeJobReader struct {
	jobs          []jobmodel.Job
	next          string
	job           *jobmodel.Job
	err           error
	getByIDErr    error
	calls         int // counts GetJobs invocations to assert cache hit/miss
	distinctNames []string
	countAllErr   error
	jobPostIDsErr error
}

func (f *fakeJobReader) GetJobs(_ context.Context, _ jobdto.JobFilter, _ string, _ int) ([]jobmodel.Job, string, error) {
	f.calls++
	return f.jobs, f.next, f.err
}
func (f *fakeJobReader) GetJobByID(_ context.Context, _ string) (*jobmodel.Job, error) {
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	return f.job, nil
}
func (f *fakeJobReader) GetJobsByIDs(_ context.Context, _ []string) ([]jobmodel.Job, error) {
	return f.jobs, nil
}
func (f *fakeJobReader) GetJobsAddedByDay(_ context.Context, _ int) ([]jobdto.DailyJobCount, error) {
	return nil, nil
}
func (f *fakeJobReader) GetJobPostIDs(_ context.Context, ids []string) ([]string, error) {
	if f.jobPostIDsErr != nil {
		return nil, f.jobPostIDsErr
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, "post-"+id)
	}
	return out, nil
}
func (f *fakeJobReader) CountAll(_ context.Context) (int64, error) {
	if f.countAllErr != nil {
		return 0, f.countAllErr
	}
	return int64(len(f.jobs)), nil
}
func (f *fakeJobReader) GetDistinctCompanyNames(_ context.Context) ([]string, error) {
	return f.distinctNames, nil
}

type fakeUserState struct {
	getResult    *jobmodel.UserJobState
	getErr       error
	manyResult   map[string]*jobmodel.UserJobState
	manyErr      error
	upsertCalls  int
	lastUpsertID string
	upsertState  jobdto.UpsertState
	bulkCalls    int
	bulkIDs      []string
	deleteCalls  int
	deleteErr    error
	countsResult jobdto.Counts
	countsErr    error
	upsertErr    error
	// SetSaved fields
	setSavedCalls    int
	lastSetSavedID   string
	lastSetSavedBool bool
	setSavedErr      error
	// GetJobsByStatus fields
	statusJobs       []jobmodel.Job
	statusNext       string
	statusErr        error
	statusCalls      int
	lastStatusStatus string
}

// strPtr returns a pointer to s. Helper for *string status fixtures.
func strPtr(s string) *string { return &s }

func (f *fakeUserState) Get(_ context.Context, userID, jobID string) (*jobmodel.UserJobState, error) {
	if userID == "" {
		return nil, apperr.ErrInternal.WithMessage("user id required")
	}
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.getResult, nil
}
func (f *fakeUserState) GetMany(_ context.Context, userID string, _ []string) (map[string]*jobmodel.UserJobState, error) {
	if userID == "" {
		return nil, apperr.ErrInternal.WithMessage("user id required")
	}
	if f.manyErr != nil {
		return nil, f.manyErr
	}
	if f.manyResult == nil {
		return map[string]*jobmodel.UserJobState{}, nil
	}
	return f.manyResult, nil
}
func (f *fakeUserState) Upsert(_ context.Context, userID, jobID string, state jobdto.UpsertState) error {
	if userID == "" {
		return apperr.ErrInternal.WithMessage("user id required")
	}
	f.upsertCalls++
	f.lastUpsertID = jobID
	f.upsertState = state
	return f.upsertErr
}
func (f *fakeUserState) UpsertBulkDismiss(_ context.Context, userID string, ids []string, _ string, _ string) error {
	if userID == "" {
		return apperr.ErrInternal.WithMessage("user id required")
	}
	f.bulkCalls++
	f.bulkIDs = ids
	return nil
}
func (f *fakeUserState) Delete(_ context.Context, userID, _ string) error {
	if userID == "" {
		return apperr.ErrInternal.WithMessage("user id required")
	}
	f.deleteCalls++
	return f.deleteErr
}
func (f *fakeUserState) CountsByUser(_ context.Context, userID string) (jobdto.Counts, error) {
	if userID == "" {
		return jobdto.Counts{}, apperr.ErrInternal.WithMessage("user id required")
	}
	return f.countsResult, f.countsErr
}
func (f *fakeUserState) SetSaved(_ context.Context, userID, jobID string, saved bool) error {
	if userID == "" {
		return apperr.ErrInternal.WithMessage("user id required")
	}
	f.setSavedCalls++
	f.lastSetSavedID = jobID
	f.lastSetSavedBool = saved
	return f.setSavedErr
}

func (f *fakeUserState) GetJobsByStatus(_ context.Context, userID string, filter jobdto.JobFilter, _ string, _ int) ([]jobmodel.Job, string, error) {
	if userID == "" {
		return nil, "", apperr.ErrInternal.WithMessage("user id required")
	}
	f.statusCalls++
	f.lastStatusStatus = filter.Status
	return f.statusJobs, f.statusNext, f.statusErr
}

type fakeSnapshot struct{}

func (fakeSnapshot) InsertSnapshot(_ context.Context, _ jobdto.SnapshotInput) error { return nil }
func (fakeSnapshot) UpdateSnapshotAction(_ context.Context, _, _ string) error      { return nil }
func (fakeSnapshot) UpdateSnapshotActionBatch(_ context.Context, _ []string, _ string) error {
	return nil
}

type fakeAppWriter struct{ calls int }

func (f *fakeAppWriter) InsertAppliedApplication(_ context.Context, _ string) error {
	f.calls++
	return nil
}

type fakeLedgerWriter struct {
	calls      int
	batchCalls int
}

func (f *fakeLedgerWriter) MarkSeen(_ context.Context, _, _, _ string) error { return nil }
func (f *fakeLedgerWriter) MarkSeenBatch(_ context.Context, _ []string, _, _ string) error {
	return nil
}
func (f *fakeLedgerWriter) UpdateSeenReason(_ context.Context, _, _ string) error {
	f.calls++
	return nil
}
func (f *fakeLedgerWriter) UpdateSeenReasonBatch(_ context.Context, _ []string, _ string) error {
	f.batchCalls++
	return nil
}
func (f *fakeLedgerWriter) UpdateRetryBatch(_ context.Context, _ []jobsvc.RetryLedgerEntry) error {
	return nil
}
func (f *fakeLedgerWriter) MarkRemovedBatch(_ context.Context, _ []string) error { return nil }

// helper — wires the impl service for tests with a memory cache.
func newSvc(reader *fakeJobReader, state *fakeUserState, app *fakeAppWriter, led *fakeLedgerWriter, c cache.Cache) jobsvc.JobService {
	return jobsvc.NewImplService(reader, state, fakeSnapshot{}, app, led, c, nil)
}

func ctxAs(userID string) context.Context {
	return identity.WithTestIdentity(context.Background(), userID)
}

// --- Tests ---

func TestImpl_List_OverlayMerge(t *testing.T) {
	now := time.Now()
	jobs := []jobmodel.Job{{ID: "j1"}, {ID: "j2"}, {ID: "j3"}}
	// In the sparse model, saved is orthogonal — SavedAt != nil signals "saved";
	// the Status workflow column remains NULL for a save-only row.
	overlay := map[string]*jobmodel.UserJobState{
		"j2": {UserID: "u1", JobID: "j2", SavedAt: &now},
	}
	reader := &fakeJobReader{jobs: jobs}
	state := &fakeUserState{manyResult: overlay}

	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, cache.NewMemoryCache())
	res, err := svc.List(ctxAs("u1"), jobdto.JobFilter{}, "", 10)
	if err != nil {
		t.Fatalf("List() err = %v", err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("want 3 items, got %d", len(res.Items))
	}
	if res.Items[0].UserState.Status != jobcommon.JobStatusNew {
		t.Errorf("j1 should overlay to new, got %q", res.Items[0].UserState.Status)
	}
	// Saved is orthogonal: a save-only row keeps view.Status="new" but populates SavedAt.
	if res.Items[1].UserState.SavedAt == nil {
		t.Errorf("j2 SavedAt should be populated, got nil")
	}
}

func TestImpl_List_SavedTab_BypassesGlobalFeed(t *testing.T) {
	// Verifies that the saved tab routes to GetJobsByStatus (user_job_state driver)
	// and does NOT use the global feed (GetJobs). Previously the saved tab fetched
	// a global page and filtered in-memory, producing sparse results.
	reader := &fakeJobReader{jobs: []jobmodel.Job{{ID: "j1"}, {ID: "j2"}}}
	state := &fakeUserState{
		statusJobs: []jobmodel.Job{{ID: "j1", Title: "Engineer", AddedAt: time.Now()}},
	}
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)

	res, err := svc.List(ctxAs("u1"), jobdto.JobFilter{Status: jobmodel.StatusSaved}, "", 10)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if reader.calls != 0 {
		t.Fatalf("GetJobs should not be called for saved tab, called %d times", reader.calls)
	}
	if len(res.Items) != 1 || res.Items[0].ID != "j1" {
		t.Fatalf("want only j1 from statusJobs, got %+v", res.Items)
	}
}

func TestImpl_List_CacheHitSkipsDB(t *testing.T) {
	jobs := []jobmodel.Job{{ID: "j1"}}
	reader := &fakeJobReader{jobs: jobs}
	state := &fakeUserState{}
	c := cache.NewMemoryCache()
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, c)

	if _, err := svc.List(ctxAs("u1"), jobdto.JobFilter{}, "", 10); err != nil {
		t.Fatalf("first List err = %v", err)
	}
	if _, err := svc.List(ctxAs("u1"), jobdto.JobFilter{}, "", 10); err != nil {
		t.Fatalf("second List err = %v", err)
	}
	if reader.calls != 1 {
		t.Errorf("expected 1 DB call (second served from cache), got %d", reader.calls)
	}
}

func TestImpl_Get_NotFound(t *testing.T) {
	svc := newSvc(&fakeJobReader{getByIDErr: gorm.ErrRecordNotFound}, &fakeUserState{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	_, err := svc.Get(ctxAs("u1"), "missing")
	var serr *apperr.ServiceError
	if !stderrors.As(err, &serr) || serr.Code != "NOT_FOUND" {
		t.Fatalf("want NOT_FOUND, got %v", err)
	}
}

func TestImpl_Get_OverlayMissingMeansNew(t *testing.T) {
	job := &jobmodel.Job{ID: "j1"}
	reader := &fakeJobReader{job: job}
	state := &fakeUserState{getErr: apperr.ErrNotFound.WithMessage("not found")}

	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	view, err := svc.Get(ctxAs("u1"), "j1")
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	if view.UserState.Status != jobcommon.JobStatusNew {
		t.Errorf("missing overlay should default to 'new', got %q", view.UserState.Status)
	}
}

func TestImpl_SetSaved_RequiresUserID(t *testing.T) {
	svc := newSvc(&fakeJobReader{}, &fakeUserState{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	if err := svc.SetSaved(context.Background(), "j1", true); err == nil {
		t.Fatal("expected error without identity in ctx")
	}
}

func TestImpl_SetSaved_RoutesThroughSetSaved(t *testing.T) {
	// SetSaved now delegates to the dedicated repo SetSaved path instead of
	// Upsert+Delete; saved is orthogonal to workflow status.
	state := &fakeUserState{}
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	if err := svc.SetSaved(ctxAs("u1"), "j1", false); err != nil {
		t.Fatalf("SetSaved err = %v", err)
	}
	if state.setSavedCalls != 1 {
		t.Errorf("expected repo.SetSaved called once, got %d", state.setSavedCalls)
	}
	if state.lastSetSavedBool != false {
		t.Errorf("lastSetSavedBool = %v, want false", state.lastSetSavedBool)
	}
	if state.deleteCalls != 0 || state.upsertCalls != 0 {
		t.Errorf("legacy paths invoked — delete=%d upsert=%d", state.deleteCalls, state.upsertCalls)
	}
}

func TestImpl_SetStatus_AppliedTriggersAppWriter(t *testing.T) {
	state := &fakeUserState{}
	app := &fakeAppWriter{}
	svc := newSvc(&fakeJobReader{}, state, app, &fakeLedgerWriter{}, nil)
	if err := svc.SetStatus(ctxAs("u1"), "j1", jobmodel.StatusApplied); err != nil {
		t.Fatalf("SetStatus err = %v", err)
	}
	if app.calls != 1 {
		t.Errorf("expected app writer to be called, got %d", app.calls)
	}
	if state.upsertState.Status != jobmodel.StatusApplied {
		t.Errorf("upsert status = %q, want %q", state.upsertState.Status, jobmodel.StatusApplied)
	}
}

func TestImpl_Dismiss_UpsertsAndUpdatesLedger(t *testing.T) {
	job := &jobmodel.Job{ID: "j1", JobPostID: "post-1"}
	state := &fakeUserState{}
	led := &fakeLedgerWriter{}
	svc := newSvc(&fakeJobReader{job: job}, state, &fakeAppWriter{}, led, nil)
	if err := svc.Dismiss(ctxAs("u1"), "j1", jobcommon.DismissalReasonUserDismissed, "ok"); err != nil {
		t.Fatalf("Dismiss err = %v", err)
	}
	if state.upsertState.Status != jobmodel.StatusDismissed {
		t.Errorf("upsert status = %q, want %q", state.upsertState.Status, jobmodel.StatusDismissed)
	}
	if led.calls != 1 {
		t.Errorf("expected ledger update, got %d", led.calls)
	}
}

func TestImpl_BulkDismiss_DelegatesAll(t *testing.T) {
	state := &fakeUserState{}
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	if err := svc.BulkDismiss(ctxAs("u1"), []string{"j1", "j2"}, jobcommon.DismissalReasonNotRelevant, ""); err != nil {
		t.Fatalf("BulkDismiss err = %v", err)
	}
	if state.bulkCalls != 1 {
		t.Errorf("expected one bulk call, got %d", state.bulkCalls)
	}
	if len(state.bulkIDs) != 2 {
		t.Errorf("expected 2 ids, got %d", len(state.bulkIDs))
	}
}

func TestImpl_GetCounts_RequiresUserID(t *testing.T) {
	svc := newSvc(&fakeJobReader{}, &fakeUserState{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	if _, err := svc.GetCounts(context.Background()); err == nil {
		t.Fatal("expected error without identity in ctx")
	}
}

func TestImpl_GetCounts_CacheHit(t *testing.T) {
	state := &fakeUserState{countsResult: jobdto.Counts{Saved: 3, Applied: 2}}
	c := cache.NewMemoryCache()
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, c)

	first, err := svc.GetCounts(ctxAs("u1"))
	if err != nil {
		t.Fatalf("first GetCounts err = %v", err)
	}
	if first.Saved != 3 {
		t.Errorf("first counts saved = %d, want 3", first.Saved)
	}
	// Mutate the underlying source — second call must come from cache.
	state.countsResult = jobdto.Counts{Saved: 99}
	second, err := svc.GetCounts(ctxAs("u1"))
	if err != nil {
		t.Fatalf("second GetCounts err = %v", err)
	}
	if second.Saved != 3 {
		t.Errorf("second counts (cached) saved = %d, want 3", second.Saved)
	}
}

func TestImpl_SetStatus_InvalidatesCountsCache(t *testing.T) {
	state := &fakeUserState{countsResult: jobdto.Counts{Applied: 1}}
	c := cache.NewMemoryCache()
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, c)

	if _, err := svc.GetCounts(ctxAs("u1")); err != nil {
		t.Fatalf("prime err = %v", err)
	}
	state.countsResult = jobdto.Counts{Applied: 5}
	if err := svc.SetStatus(ctxAs("u1"), "j1", jobmodel.StatusApplied); err != nil {
		t.Fatalf("SetStatus err = %v", err)
	}
	got, err := svc.GetCounts(ctxAs("u1"))
	if err != nil {
		t.Fatalf("GetCounts err = %v", err)
	}
	if got.Applied != 5 {
		t.Errorf("after invalidation got = %d, want 5", got.Applied)
	}
}

func TestList_SavedTab_RoutesToGetJobsByStatus(t *testing.T) {
	now := time.Now()
	reader := &fakeJobReader{}
	// Saved overlay rows carry SavedAt != nil and a NULL workflow status — the
	// view projects this to "new" + SavedAt populated; the saved tab routing
	// itself is verified by statusCalls/lastStatusStatus.
	state := &fakeUserState{
		statusJobs: []jobmodel.Job{
			{ID: "j1", Title: "Engineer", AddedAt: time.Now()},
		},
		manyResult: map[string]*jobmodel.UserJobState{
			"j1": {JobID: "j1", SavedAt: &now},
		},
	}
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	ctx := ctxAs("u1")

	result, err := svc.List(ctx, jobdto.JobFilter{Status: "saved"}, "", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.statusCalls != 1 {
		t.Errorf("expected GetJobsByStatus called once, got %d", state.statusCalls)
	}
	if reader.calls != 0 {
		t.Errorf("expected GetJobs not called for saved tab, got %d calls", reader.calls)
	}
	if state.lastStatusStatus != "saved" {
		t.Errorf("expected status=saved, got %q", state.lastStatusStatus)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item from saved tab, got %+v", result.Items)
	}
	if result.Items[0].UserState.SavedAt == nil {
		t.Errorf("expected SavedAt populated on saved-tab view, got nil")
	}
}

func TestList_NewTab_UsesGlobalFeed(t *testing.T) {
	reader := &fakeJobReader{
		jobs: []jobmodel.Job{{ID: "j1", Title: "Engineer", AddedAt: time.Now()}},
	}
	state := &fakeUserState{}
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	ctx := ctxAs("u1")

	_, err := svc.List(ctx, jobdto.JobFilter{Status: "new"}, "", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.statusCalls != 0 {
		t.Errorf("expected GetJobsByStatus not called for new tab, got %d", state.statusCalls)
	}
	if reader.calls != 1 {
		t.Errorf("expected GetJobs called once for new tab, got %d", reader.calls)
	}
}

func TestList_AppliedTab_SetsStatusOnAllViews(t *testing.T) {
	state := &fakeUserState{
		statusJobs: []jobmodel.Job{
			{ID: "j1", Title: "SWE", AddedAt: time.Now()},
			{ID: "j2", Title: "SRE", AddedAt: time.Now().Add(-time.Hour)},
		},
		manyResult: map[string]*jobmodel.UserJobState{
			"j1": {JobID: "j1", Status: strPtr(jobcommon.JobStatusApplied)},
			"j2": {JobID: "j2", Status: strPtr(jobcommon.JobStatusApplied)},
		},
	}
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	ctx := ctxAs("u1")

	result, err := svc.List(ctx, jobdto.JobFilter{Status: "applied"}, "", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, item := range result.Items {
		if item.UserState.Status != "applied" {
			t.Errorf("expected status=applied on all views, got %q on item %s", item.UserState.Status, item.ID)
		}
	}
}

// -------- Status routing across every status value --------

// TestImpl_List_StatusRouting verifies every status value routes to the right
// underlying store method. Regressions here can drop or duplicate rows.
func TestImpl_List_StatusRouting(t *testing.T) {
	tests := []struct {
		status              string
		wantGlobal          bool // jobs reader called
		wantUserStateStatus bool // GetJobsByStatus called
	}{
		{status: "", wantGlobal: true},
		{status: jobcommon.JobStatusNew, wantGlobal: true},
		{status: jobcommon.JobStatusSaved, wantUserStateStatus: true},
		{status: jobcommon.JobStatusApplied, wantUserStateStatus: true},
		{status: jobcommon.JobStatusInterviewing, wantUserStateStatus: true},
		{status: jobcommon.JobStatusOffer, wantUserStateStatus: true},
		{status: jobcommon.JobStatusRejected, wantUserStateStatus: true},
		{status: jobcommon.JobStatusDismissed, wantUserStateStatus: true},
	}
	for _, tc := range tests {
		t.Run("status="+tc.status, func(t *testing.T) {
			reader := &fakeJobReader{}
			state := &fakeUserState{}
			svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
			if _, err := svc.List(ctxAs("u1"), jobdto.JobFilter{Status: tc.status}, "", 10); err != nil {
				t.Fatalf("List err = %v", err)
			}
			if tc.wantGlobal && reader.calls != 1 {
				t.Errorf("GetJobs calls = %d, want 1", reader.calls)
			}
			if !tc.wantGlobal && reader.calls != 0 {
				t.Errorf("GetJobs calls = %d, want 0", reader.calls)
			}
			if tc.wantUserStateStatus && state.statusCalls != 1 {
				t.Errorf("GetJobsByStatus calls = %d, want 1", state.statusCalls)
			}
			if !tc.wantUserStateStatus && state.statusCalls != 0 {
				t.Errorf("GetJobsByStatus calls = %d, want 0", state.statusCalls)
			}
		})
	}
}

// TestImpl_List_NewTab_FiltersOutNonNewOverlay confirms the documented sparse-
// pages behaviour: when a job has a non-new overlay (saved/applied/etc.), the
// new-tab merge skips it. Pages may have fewer items than the limit, but the
// next_cursor advances. Frontend pagination must tolerate sparse pages.
func TestImpl_List_NewTab_FiltersOutNonNewOverlay(t *testing.T) {
	now := time.Now()
	jobs := []jobmodel.Job{{ID: "j1"}, {ID: "j2"}, {ID: "j3"}}
	overlay := map[string]*jobmodel.UserJobState{
		// j1: saved-only — saved is orthogonal but still excludes from new tab.
		"j1": {UserID: "u1", JobID: "j1", SavedAt: &now},
		// j2: dismissed workflow status — excluded from new tab.
		"j2": {UserID: "u1", JobID: "j2", Status: strPtr(jobmodel.StatusDismissed)},
		// j3 has no overlay → implicitly "new"
	}
	reader := &fakeJobReader{jobs: jobs}
	state := &fakeUserState{manyResult: overlay}
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)

	res, err := svc.List(ctxAs("u1"), jobdto.JobFilter{Status: jobcommon.JobStatusNew}, "", 10)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].ID != "j3" {
		t.Fatalf("want only j3 on new tab, got %+v", res.Items)
	}
	if res.Items[0].UserState.Status != jobcommon.JobStatusNew {
		t.Errorf("j3 status = %q, want new", res.Items[0].UserState.Status)
	}
}

// TestImpl_List_NoStatus_NoOverlayFilter — status="" means "any" — every job
// is returned regardless of overlay. This is the contract that powers admin
// or unfiltered views; tested to lock it in.
func TestImpl_List_NoStatus_NoOverlayFilter(t *testing.T) {
	now := time.Now()
	jobs := []jobmodel.Job{{ID: "j1"}, {ID: "j2"}, {ID: "j3"}}
	overlay := map[string]*jobmodel.UserJobState{
		"j1": {UserID: "u1", JobID: "j1", SavedAt: &now},
		"j2": {UserID: "u1", JobID: "j2", Status: strPtr(jobmodel.StatusDismissed)},
	}
	reader := &fakeJobReader{jobs: jobs}
	state := &fakeUserState{manyResult: overlay}
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)

	res, err := svc.List(ctxAs("u1"), jobdto.JobFilter{}, "", 10)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(res.Items) != 3 {
		t.Errorf("want 3 items (no status filter), got %d", len(res.Items))
	}
}

// -------- Cache behaviour --------

// TestImpl_List_CacheKey_IgnoresStatus — the cache key excludes the status
// filter so two callers asking for the same filter but different status tabs
// share the same DB read (then apply the overlay per-user).
func TestImpl_List_CacheKey_IgnoresStatus(t *testing.T) {
	jobs := []jobmodel.Job{{ID: "j1"}}
	reader := &fakeJobReader{jobs: jobs}
	state := &fakeUserState{}
	c := cache.NewMemoryCache()
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, c)
	ctx := ctxAs("u1")

	if _, err := svc.List(ctx, jobdto.JobFilter{Status: "new"}, "", 10); err != nil {
		t.Fatalf("first List: %v", err)
	}
	if _, err := svc.List(ctx, jobdto.JobFilter{Status: ""}, "", 10); err != nil {
		t.Fatalf("second List: %v", err)
	}
	if reader.calls != 1 {
		t.Errorf("DB calls = %d, want 1 (status not part of cache key)", reader.calls)
	}
}

// TestImpl_List_CacheKey_DifferentFilterTriggersRefetch — different search /
// companies / experience filters must produce different cache keys.
func TestImpl_List_CacheKey_DifferentFilterTriggersRefetch(t *testing.T) {
	jobs := []jobmodel.Job{{ID: "j1"}}
	reader := &fakeJobReader{jobs: jobs}
	state := &fakeUserState{}
	c := cache.NewMemoryCache()
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, c)
	ctx := ctxAs("u1")

	calls := []jobdto.JobFilter{
		{Search: "engineer"},
		{Search: "designer"},
		{Companies: []string{"Google"}},
		{Companies: []string{"Meta"}},
	}
	for _, f := range calls {
		if _, err := svc.List(ctx, f, "", 10); err != nil {
			t.Fatalf("List err = %v", err)
		}
	}
	if reader.calls != len(calls) {
		t.Errorf("DB calls = %d, want %d (every distinct filter refetches)", reader.calls, len(calls))
	}
}

// TestImpl_List_CacheKey_SharedAcrossUsers — page cache is intentionally
// user-agnostic. Two users hitting the same filter share the cached page
// and each applies their own overlay.
func TestImpl_List_CacheKey_SharedAcrossUsers(t *testing.T) {
	jobs := []jobmodel.Job{{ID: "j1"}}
	reader := &fakeJobReader{jobs: jobs}
	state := &fakeUserState{}
	c := cache.NewMemoryCache()
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, c)

	if _, err := svc.List(ctxAs("u1"), jobdto.JobFilter{}, "", 10); err != nil {
		t.Fatalf("u1 List: %v", err)
	}
	if _, err := svc.List(ctxAs("u2"), jobdto.JobFilter{}, "", 10); err != nil {
		t.Fatalf("u2 List: %v", err)
	}
	if reader.calls != 1 {
		t.Errorf("DB calls = %d, want 1 (page cache shared between users)", reader.calls)
	}
}

// TestImpl_List_StatusTab_NoCache — status tabs other than "new" never use
// the page cache (driven from user_job_state directly).
func TestImpl_List_StatusTab_NoCache(t *testing.T) {
	state := &fakeUserState{statusJobs: []jobmodel.Job{{ID: "j1"}}}
	c := cache.NewMemoryCache()
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, c)
	ctx := ctxAs("u1")

	if _, err := svc.List(ctx, jobdto.JobFilter{Status: "saved"}, "", 10); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := svc.List(ctx, jobdto.JobFilter{Status: "saved"}, "", 10); err != nil {
		t.Fatalf("second: %v", err)
	}
	if state.statusCalls != 2 {
		t.Errorf("GetJobsByStatus calls = %d, want 2 (no caching on status tabs)", state.statusCalls)
	}
}

// -------- Mutation cache invalidation --------

// TestImpl_Mutations_InvalidateCountsCache asserts every mutating method
// invalidates the per-user counts cache. Stale counts after a write surfaces
// to users as wrong tab badges.
func TestImpl_Mutations_InvalidateCountsCache(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(svc jobsvc.JobService, ctx context.Context) error
	}{
		{name: "SetSaved true", mutate: func(svc jobsvc.JobService, ctx context.Context) error {
			return svc.SetSaved(ctx, "j1", true)
		}},
		{name: "SetSaved false", mutate: func(svc jobsvc.JobService, ctx context.Context) error {
			return svc.SetSaved(ctx, "j1", false)
		}},
		{name: "SetStatus applied", mutate: func(svc jobsvc.JobService, ctx context.Context) error {
			return svc.SetStatus(ctx, "j1", jobmodel.StatusApplied)
		}},
		{name: "BulkDismiss", mutate: func(svc jobsvc.JobService, ctx context.Context) error {
			return svc.BulkDismiss(ctx, []string{"j1"}, jobcommon.DismissalReasonNotRelevant, "")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := &fakeUserState{countsResult: jobdto.Counts{Saved: 1}}
			c := cache.NewMemoryCache()
			svc := newSvc(&fakeJobReader{job: &jobmodel.Job{ID: "j1"}}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, c)
			ctx := ctxAs("u1")

			// Prime cache.
			if _, err := svc.GetCounts(ctx); err != nil {
				t.Fatalf("prime: %v", err)
			}
			// Mutate underlying source so we can tell stale from fresh.
			state.countsResult = jobdto.Counts{Saved: 99}
			if err := tc.mutate(svc, ctx); err != nil {
				t.Fatalf("mutate: %v", err)
			}
			// Next read should see the new value, not the cached 1.
			got, err := svc.GetCounts(ctx)
			if err != nil {
				t.Fatalf("post-mutate GetCounts: %v", err)
			}
			if got.Saved != 99 {
				t.Errorf("got.Saved = %d, want 99 (cache should have been invalidated)", got.Saved)
			}
		})
	}
}

// TestImpl_Dismiss_InvalidatesCountsCache covers single Dismiss separately
// because it requires the job to exist (jobsReader.job is populated).
func TestImpl_Dismiss_InvalidatesCountsCache(t *testing.T) {
	state := &fakeUserState{countsResult: jobdto.Counts{Dismissed: 1}}
	reader := &fakeJobReader{job: &jobmodel.Job{ID: "j1", JobPostID: "post-1"}}
	c := cache.NewMemoryCache()
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, c)
	ctx := ctxAs("u1")

	if _, err := svc.GetCounts(ctx); err != nil {
		t.Fatalf("prime: %v", err)
	}
	state.countsResult = jobdto.Counts{Dismissed: 99}
	if err := svc.Dismiss(ctx, "j1", jobcommon.DismissalReasonUserDismissed, ""); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}
	got, err := svc.GetCounts(ctx)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if got.Dismissed != 99 {
		t.Errorf("Dismissed = %d, want 99 (cache invalidated)", got.Dismissed)
	}
}

// -------- Missing user-id rejection --------

// TestImpl_MutationsRequireUserID asserts every mutating method errors when
// the context carries no identity. This is the defence-in-depth against a
// forgotten ctx scoping; it complements the per-repo userScoped guards.
func TestImpl_MutationsRequireUserID(t *testing.T) {
	svc := newSvc(&fakeJobReader{}, &fakeUserState{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	ctx := context.Background()
	tests := []struct {
		name string
		call func() error
	}{
		{name: "SetSaved", call: func() error { return svc.SetSaved(ctx, "j1", true) }},
		{name: "SetStatus", call: func() error { return svc.SetStatus(ctx, "j1", "applied") }},
		{name: "SetNotes", call: func() error { return svc.SetNotes(ctx, "j1", "n") }},
		{name: "Dismiss", call: func() error { return svc.Dismiss(ctx, "j1", "user_dismissed", "") }},
		{name: "BulkDismiss", call: func() error { return svc.BulkDismiss(ctx, []string{"j1"}, "user_not_relevant", "") }},
		{name: "GetCounts", call: func() error { _, err := svc.GetCounts(ctx); return err }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatalf("%s with no identity must error", tc.name)
			}
		})
	}
}

// -------- SetNotes status preservation --------

func TestImpl_SetNotes_ExistingRowPreservesStatus(t *testing.T) {
	state := &fakeUserState{
		getResult: &jobmodel.UserJobState{Status: strPtr(jobmodel.StatusApplied)},
	}
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	if err := svc.SetNotes(ctxAs("u1"), "j1", "my note"); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}
	if state.upsertState.Status != jobmodel.StatusApplied {
		t.Errorf("status preserved = %q, want applied", state.upsertState.Status)
	}
	if state.upsertState.Notes == nil || *state.upsertState.Notes != "my note" {
		t.Errorf("notes = %v, want &\"my note\"", state.upsertState.Notes)
	}
}

func TestImpl_SetNotes_MissingRowDefaultsToSaved(t *testing.T) {
	state := &fakeUserState{
		getErr: apperr.ErrNotFound.WithMessage("not found"),
	}
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	if err := svc.SetNotes(ctxAs("u1"), "j1", "first note"); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}
	if state.upsertState.Status != jobmodel.StatusSaved {
		t.Errorf("default status = %q, want saved", state.upsertState.Status)
	}
}

// TestImpl_SetNotes_FirstNote_InvalidatesCountsCache pins the behaviour that
// the implicit "new" → "saved" transition triggered by the first note on a
// job clears the per-user counts cache. Without the invalidation, "new"
// would over-count and "saved" would under-count until the TTL elapsed.
func TestImpl_SetNotes_FirstNote_InvalidatesCountsCache(t *testing.T) {
	state := &fakeUserState{
		getErr:       apperr.ErrNotFound.WithMessage("not found"),
		countsResult: jobdto.Counts{Saved: 1},
	}
	c := cache.NewMemoryCache()
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, c)
	ctx := ctxAs("u1")

	if _, err := svc.GetCounts(ctx); err != nil {
		t.Fatalf("prime: %v", err)
	}
	state.countsResult = jobdto.Counts{Saved: 99}
	if err := svc.SetNotes(ctx, "j1", "first note"); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}
	got, err := svc.GetCounts(ctx)
	if err != nil {
		t.Fatalf("post-mutate GetCounts: %v", err)
	}
	if got.Saved != 99 {
		t.Errorf("got.Saved = %d, want 99 (cache should be invalidated when a new saved row is created)", got.Saved)
	}
}

// TestImpl_SetNotes_ExistingRow_DoesNotInvalidateCountsCache locks in the
// inverse: editing the note on a row that already exists does not shift any
// status counter, so the saved/applied/dismissed totals are unchanged and
// we should not pay a wasted DELETE on every keystroke.
func TestImpl_SetNotes_ExistingRow_DoesNotInvalidateCountsCache(t *testing.T) {
	state := &fakeUserState{
		getResult:    &jobmodel.UserJobState{Status: strPtr(jobmodel.StatusApplied)},
		countsResult: jobdto.Counts{Applied: 1},
	}
	c := cache.NewMemoryCache()
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, c)
	ctx := ctxAs("u1")

	if _, err := svc.GetCounts(ctx); err != nil {
		t.Fatalf("prime: %v", err)
	}
	// Shift the underlying state. If SetNotes invalidates, the next read
	// would see 99; if it doesn't, it returns the cached 1 — which is what
	// we want, because the user's actual count didn't change.
	state.countsResult = jobdto.Counts{Applied: 99}
	if err := svc.SetNotes(ctx, "j1", "more thoughts"); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}
	got, err := svc.GetCounts(ctx)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if got.Applied != 1 {
		t.Errorf("got.Applied = %d, want 1 (re-edit must not invalidate)", got.Applied)
	}
}

// -------- GetCounts derivation --------

// TestImpl_GetCounts_DerivesNew confirms New = CountAll - sum(others).
func TestImpl_GetCounts_DerivesNew(t *testing.T) {
	reader := &fakeJobReader{
		jobs: []jobmodel.Job{
			{ID: "j1"}, {ID: "j2"}, {ID: "j3"}, {ID: "j4"}, {ID: "j5"},
			{ID: "j6"}, {ID: "j7"}, {ID: "j8"}, {ID: "j9"}, {ID: "j10"},
		}, // CountAll = 10
	}
	state := &fakeUserState{countsResult: jobdto.Counts{
		Saved: 2, Applied: 1, Dismissed: 3, Interviewing: 1, Offer: 0, Rejected: 0,
	}} // sum = 7
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	got, err := svc.GetCounts(ctxAs("u1"))
	if err != nil {
		t.Fatalf("GetCounts: %v", err)
	}
	if got.New != 3 {
		t.Errorf("New = %d, want 3 (10 - 7)", got.New)
	}
}

// TestImpl_GetCounts_CountAllUnderflow — when sum(others) > total, New must
// stay 0 (never go negative). Defensive guard against snapshot/state drift.
func TestImpl_GetCounts_CountAllUnderflow(t *testing.T) {
	reader := &fakeJobReader{jobs: []jobmodel.Job{{ID: "j1"}}} // CountAll = 1
	state := &fakeUserState{countsResult: jobdto.Counts{Saved: 5}}
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	got, err := svc.GetCounts(ctxAs("u1"))
	if err != nil {
		t.Fatalf("GetCounts: %v", err)
	}
	if got.New != 0 {
		t.Errorf("New = %d, want 0 (underflow guarded)", got.New)
	}
}

// -------- BulkDismiss edge cases --------

// TestImpl_BulkDismiss_EmptyIDs_NoOp — impl returns nil without touching the
// store. Validation rejects this case earlier, but the impl is hardened so
// programmatic callers don't trigger a needless repo write.
func TestImpl_BulkDismiss_EmptyIDs_NoOp(t *testing.T) {
	state := &fakeUserState{}
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	if err := svc.BulkDismiss(ctxAs("u1"), nil, jobcommon.DismissalReasonNotRelevant, ""); err != nil {
		t.Fatalf("BulkDismiss: %v", err)
	}
	if state.bulkCalls != 0 {
		t.Errorf("bulkCalls = %d, want 0 for empty IDs", state.bulkCalls)
	}
}

// -------- Dismiss edge cases --------

// TestImpl_Dismiss_JobNotFound returns the typed NOT_FOUND service error so
// the handler maps it to 404.
func TestImpl_Dismiss_JobNotFound(t *testing.T) {
	svc := newSvc(&fakeJobReader{getByIDErr: gorm.ErrRecordNotFound},
		&fakeUserState{}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	err := svc.Dismiss(ctxAs("u1"), "missing", jobcommon.DismissalReasonUserDismissed, "")
	var serr *apperr.ServiceError
	if !stderrors.As(err, &serr) || serr.Code != "NOT_FOUND" {
		t.Fatalf("err = %v, want NOT_FOUND ServiceError", err)
	}
}

// TestImpl_Dismiss_EmptyNoteNotStored — when note is "" the impl passes a nil
// DismissalNote so the column is not overwritten with an empty string.
func TestImpl_Dismiss_EmptyNoteNotStored(t *testing.T) {
	state := &fakeUserState{}
	reader := &fakeJobReader{job: &jobmodel.Job{ID: "j1", JobPostID: "post-1"}}
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	if err := svc.Dismiss(ctxAs("u1"), "j1", jobcommon.DismissalReasonUserDismissed, ""); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}
	if state.upsertState.DismissalNote != nil {
		t.Errorf("DismissalNote = %v, want nil (empty note)", state.upsertState.DismissalNote)
	}
	if state.upsertState.DismissalReason == nil || *state.upsertState.DismissalReason != jobcommon.DismissalReasonUserDismissed {
		t.Errorf("DismissalReason = %v, want user_dismissed", state.upsertState.DismissalReason)
	}
}

// -------- Get + List behaviour with no user identity --------

// TestImpl_Get_NoIdentity returns the job without per-user overlay (anonymous
// reads default to status="new"). Important for callers that surface job
// detail without an authenticated user.
func TestImpl_Get_NoIdentity(t *testing.T) {
	reader := &fakeJobReader{job: &jobmodel.Job{ID: "j1"}}
	state := &fakeUserState{}
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	view, err := svc.Get(context.Background(), "j1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if view.UserState.Status != jobcommon.JobStatusNew {
		t.Errorf("anonymous status = %q, want new", view.UserState.Status)
	}
}

// TestImpl_List_NoIdentity returns the global feed without overlay. Catches a
// regression where the impl crashes (or upstream blocks) on anonymous reads.
func TestImpl_List_NoIdentity(t *testing.T) {
	reader := &fakeJobReader{jobs: []jobmodel.Job{{ID: "j1"}}}
	state := &fakeUserState{}
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	res, err := svc.List(context.Background(), jobdto.JobFilter{}, "", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].UserState.Status != jobcommon.JobStatusNew {
		t.Errorf("res = %+v, want one item status=new", res.Items)
	}
}

// Compile-time interface checks.
var _ jobsvc.JobReader = (*fakeJobReader)(nil)
var _ jobsvc.UserJobStateRepository = (*fakeUserState)(nil)
var _ jobsvc.SnapshotWriter = fakeSnapshot{}
var _ jobsvc.ApplicationWriter = (*fakeAppWriter)(nil)
var _ jobsvc.SeenLedgerWriter = (*fakeLedgerWriter)(nil)
