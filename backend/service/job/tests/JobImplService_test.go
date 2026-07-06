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
	lastStatusFilter jobdto.JobFilter
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
	f.lastStatusFilter = filter
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
func (f *fakeLedgerWriter) UpdateSeenReason(_ context.Context, _ string, _ jobcommon.SeenReason) error {
	f.calls++
	return nil
}
func (f *fakeLedgerWriter) UpdateSeenReasonBatch(_ context.Context, _ []string, _ jobcommon.SeenReason) error {
	f.batchCalls++
	return nil
}
func (f *fakeLedgerWriter) UpdateRetryBatch(_ context.Context, _ []jobsvc.RetryLedgerEntry) error {
	return nil
}
func (f *fakeLedgerWriter) UpdateRetryBatchWithPayload(_ context.Context, _ []jobsvc.RetryLedgerEntryWithPayload) error {
	return nil
}
func (f *fakeLedgerWriter) FetchDueRetryJobs(_ context.Context, _ int) ([]jobsvc.RetryJobRecord, error) {
	return nil, nil
}
func (f *fakeLedgerWriter) FetchDueRetryJobsWithPayload(_ context.Context, _ int) ([]jobsvc.RetryJobRecord, error) {
	return nil, nil
}
func (f *fakeLedgerWriter) ResetRetryToFetch(_ context.Context, _ string) error    { return nil }
func (f *fakeLedgerWriter) EscalateRetryToError(_ context.Context, _ string) error { return nil }
func (f *fakeLedgerWriter) EscalateStuckRetryJobs(_ context.Context, _ time.Duration, _ int) (int64, error) {
	return 0, nil
}
func (f *fakeLedgerWriter) ReclaimStuckFetchRows(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (f *fakeLedgerWriter) ReclaimStuckFilteredRows(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
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

// TestImpl_List_StatusesFilter_RoutesToUserStateAndForwardsFilter proves the
// dispatch-routing fix: a request with only filter.Statuses set (and
// filter.Status empty) must route to listByUserStatus/GetJobsByStatus, not
// the global "new"-feed cache path. This is the single most important test
// in the combined-status ("Tracking" tab) feature — without the dispatch
// fix, a request with only ?statuses= silently falls through to the global
// feed and GetJobsByStatus is never called, defeating the feature entirely.
func TestImpl_List_StatusesFilter_RoutesToUserStateAndForwardsFilter(t *testing.T) {
	reader := &fakeJobReader{jobs: []jobmodel.Job{{ID: "j1"}}}
	state := &fakeUserState{
		statusJobs: []jobmodel.Job{{ID: "j-applied"}, {ID: "j-interviewing"}},
	}
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)

	statuses := []string{jobcommon.JobStatusApplied, jobcommon.JobStatusInterviewing}
	res, err := svc.List(ctxAs("u1"), jobdto.JobFilter{Statuses: statuses}, "", 10)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if reader.calls != 0 {
		t.Errorf("GetJobs calls = %d, want 0 (must not hit the global feed)", reader.calls)
	}
	if state.statusCalls != 1 {
		t.Fatalf("GetJobsByStatus calls = %d, want 1", state.statusCalls)
	}
	if len(state.lastStatusFilter.Statuses) != 2 ||
		state.lastStatusFilter.Statuses[0] != jobcommon.JobStatusApplied ||
		state.lastStatusFilter.Statuses[1] != jobcommon.JobStatusInterviewing {
		t.Errorf("forwarded Statuses = %v, want %v", state.lastStatusFilter.Statuses, statuses)
	}
	if len(res.Items) != 2 {
		t.Errorf("items = %d, want 2 (from statusJobs)", len(res.Items))
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

// TestImpl_List_NewTab_FiltersOutWorkflowOverlay confirms the new-tab merge
// excludes ONLY rows with a workflow status (applied/dismissed/etc.). Saved is
// orthogonal — a saved-only row stays in the new tab AND simultaneously appears
// in the saved tab. Bookmarking a job must never silently move it out of new.
func TestImpl_List_NewTab_FiltersOutWorkflowOverlay(t *testing.T) {
	now := time.Now()
	jobs := []jobmodel.Job{{ID: "j1"}, {ID: "j2"}, {ID: "j3"}}
	overlay := map[string]*jobmodel.UserJobState{
		// j1: saved-only — orthogonal flag, must STAY on the new tab.
		"j1": {UserID: "u1", JobID: "j1", SavedAt: &now},
		// j2: dismissed workflow status — excluded from new tab.
		"j2": {UserID: "u1", JobID: "j2", Status: strPtr(jobmodel.StatusDismissed)},
		// j3: no overlay → implicitly "new".
	}
	reader := &fakeJobReader{jobs: jobs}
	state := &fakeUserState{manyResult: overlay}
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)

	res, err := svc.List(ctxAs("u1"), jobdto.JobFilter{Status: jobcommon.JobStatusNew}, "", 10)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("want 2 items on new tab (j1 saved-only + j3 untouched), got %d: %+v",
			len(res.Items), res.Items)
	}
	got := map[string]bool{}
	for _, it := range res.Items {
		got[it.ID] = true
		if it.UserState.Status != jobcommon.JobStatusNew {
			t.Errorf("%s status = %q, want new", it.ID, it.UserState.Status)
		}
	}
	if !got["j1"] || !got["j3"] {
		t.Fatalf("missing expected ids j1, j3 in result: %+v", res.Items)
	}
	// j1 must surface saved_at populated so the bookmark icon stays filled
	// on the new tab.
	for _, it := range res.Items {
		if it.ID == "j1" && it.UserState.SavedAt == nil {
			t.Error("j1 SavedAt should be populated on new tab (orthogonal flag)")
		}
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
