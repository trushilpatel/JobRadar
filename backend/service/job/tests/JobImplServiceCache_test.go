package services_test

import (
	"context"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/platform/cache"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// Cache-behavior tests (page cache + per-user counts cache, including
// invalidation on every mutating method) split out of JobImplService_test.go
// to keep hot-path test files under the 800-line ergonomics threshold.
// Shared fixtures (fakeJobReader, fakeUserState, newSvc, ctxAs, ...) live in
// JobImplService_test.go — same package, no import needed.

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
	// Saved is orthogonal — must NOT subtract from New. The sum is over
	// workflow statuses only: applied + dismissed + interviewing + offer + rejected = 5.
	state := &fakeUserState{countsResult: jobdto.Counts{
		Saved: 2, Applied: 1, Dismissed: 3, Interviewing: 1, Offer: 0, Rejected: 0,
	}}
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	got, err := svc.GetCounts(ctxAs("u1"))
	if err != nil {
		t.Fatalf("GetCounts: %v", err)
	}
	if got.New != 5 {
		t.Errorf("New = %d, want 5 (10 - 5 workflow; saved is orthogonal)", got.New)
	}
}

// TestImpl_GetCounts_SavedDoesNotShrinkNew — bookmarking jobs must never
// lower the new-tab badge. With only Saved populated (no workflow status),
// New must equal total.
func TestImpl_GetCounts_SavedDoesNotShrinkNew(t *testing.T) {
	reader := &fakeJobReader{jobs: []jobmodel.Job{
		{ID: "j1"}, {ID: "j2"}, {ID: "j3"}, {ID: "j4"}, {ID: "j5"},
	}} // CountAll = 5
	state := &fakeUserState{countsResult: jobdto.Counts{Saved: 5}}
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	got, err := svc.GetCounts(ctxAs("u1"))
	if err != nil {
		t.Fatalf("GetCounts: %v", err)
	}
	if got.New != 5 {
		t.Errorf("New = %d, want 5 (saved is orthogonal — must not subtract)", got.New)
	}
}

// TestImpl_GetCounts_CountAllUnderflow — when sum(workflow statuses) > total,
// New must stay 0 (never go negative). Defensive guard against snapshot/state
// drift between user_job_state and the global jobs table.
func TestImpl_GetCounts_CountAllUnderflow(t *testing.T) {
	reader := &fakeJobReader{jobs: []jobmodel.Job{{ID: "j1"}}} // CountAll = 1
	state := &fakeUserState{countsResult: jobdto.Counts{Applied: 3, Dismissed: 2}}
	svc := newSvc(reader, state, &fakeAppWriter{}, &fakeLedgerWriter{}, nil)
	got, err := svc.GetCounts(ctxAs("u1"))
	if err != nil {
		t.Fatalf("GetCounts: %v", err)
	}
	if got.New != 0 {
		t.Errorf("New = %d, want 0 (underflow guarded)", got.New)
	}
}
