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
	jobs       []jobmodel.Job
	next       string
	job        *jobmodel.Job
	err        error
	getByIDErr error
	calls      int // counts GetJobs invocations to assert cache hit/miss
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
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, "post-"+id)
	}
	return out, nil
}
func (f *fakeJobReader) CountAll(_ context.Context) (int64, error) { return int64(len(f.jobs)), nil }

type fakeUserState struct {
	getResult    *jobmodel.UserJobState
	getErr       error
	manyResult   map[string]*jobmodel.UserJobState
	upsertCalls  int
	lastUpsertID string
	upsertState  jobdto.UpsertState
	bulkCalls    int
	bulkIDs      []string
	deleteCalls  int
	countsResult jobdto.Counts
	countsErr    error
	upsertErr    error
}

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
	return nil
}
func (f *fakeUserState) CountsByUser(_ context.Context, userID string) (jobdto.Counts, error) {
	if userID == "" {
		return jobdto.Counts{}, apperr.ErrInternal.WithMessage("user id required")
	}
	return f.countsResult, f.countsErr
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

type fakeLedgerWriter struct{ calls int }

func (f *fakeLedgerWriter) MarkSeen(_ context.Context, _, _, _ string) error { return nil }
func (f *fakeLedgerWriter) MarkSeenBatch(_ context.Context, _ []string, _, _ string) error {
	return nil
}
func (f *fakeLedgerWriter) UpdateSeenReason(_ context.Context, _, _ string) error {
	f.calls++
	return nil
}
func (f *fakeLedgerWriter) UpdateSeenReasonBatch(_ context.Context, _ []string, _ string) error {
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
	overlay := map[string]*jobmodel.UserJobState{
		"j2": {UserID: "u1", JobID: "j2", Status: jobmodel.StatusSaved, SavedAt: &now},
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
	if res.Items[1].UserState.Status != jobmodel.StatusSaved {
		t.Errorf("j2 should overlay to saved, got %q", res.Items[1].UserState.Status)
	}
}

func TestImpl_List_StatusFilterAppliedAfterMerge(t *testing.T) {
	now := time.Now()
	jobs := []jobmodel.Job{{ID: "j1"}, {ID: "j2"}}
	overlay := map[string]*jobmodel.UserJobState{
		"j1": {Status: jobmodel.StatusSaved, SavedAt: &now},
	}
	svc := newSvc(&fakeJobReader{jobs: jobs}, &fakeUserState{manyResult: overlay}, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)

	res, err := svc.List(ctxAs("u1"), jobdto.JobFilter{Status: jobmodel.StatusSaved}, "", 10)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].ID != "j1" {
		t.Fatalf("want only j1, got %+v", res.Items)
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

func TestImpl_SetSaved_DeletesWhenFalse(t *testing.T) {
	state := &fakeUserState{}
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	if err := svc.SetSaved(ctxAs("u1"), "j1", false); err != nil {
		t.Fatalf("SetSaved err = %v", err)
	}
	if state.deleteCalls != 1 {
		t.Errorf("expected Delete to be called, got %d", state.deleteCalls)
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
	state := &fakeUserState{countsResult: jobdto.Counts{Saved: 1}}
	c := cache.NewMemoryCache()
	svc := newSvc(&fakeJobReader{}, state, &fakeAppWriter{}, &fakeLedgerWriter{}, c)

	if _, err := svc.GetCounts(ctxAs("u1")); err != nil {
		t.Fatalf("prime err = %v", err)
	}
	state.countsResult = jobdto.Counts{Saved: 5}
	if err := svc.SetStatus(ctxAs("u1"), "j1", jobmodel.StatusSaved); err != nil {
		t.Fatalf("SetStatus err = %v", err)
	}
	got, err := svc.GetCounts(ctxAs("u1"))
	if err != nil {
		t.Fatalf("GetCounts err = %v", err)
	}
	if got.Saved != 5 {
		t.Errorf("after invalidation got = %d, want 5", got.Saved)
	}
}

// Compile-time interface checks.
var _ jobsvc.JobReader = (*fakeJobReader)(nil)
var _ jobsvc.UserJobStateRepository = (*fakeUserState)(nil)
var _ jobsvc.SnapshotWriter = fakeSnapshot{}
var _ jobsvc.ApplicationWriter = (*fakeAppWriter)(nil)
var _ jobsvc.SeenLedgerWriter = (*fakeLedgerWriter)(nil)
