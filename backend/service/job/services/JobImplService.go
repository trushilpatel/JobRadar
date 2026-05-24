package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/platform/cache"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
)

// Cache TTLs and key prefixes for the job feed. Page entries hold the raw
// list of jobs (no per-user state); the per-user overlay is merged AFTER
// the cache hit. Per-user counts cache separately — every mutating method
// invalidates the key explicitly, so the TTL is the upper bound on how
// long a missed invalidation (e.g. a Redis hiccup) can survive.
const (
	feedPageCacheTTL = 30 * time.Second
	userCountsTTL    = 60 * time.Second

	feedPageCachePrefix = "feed:page:"
	userCountsPrefix    = "user:"
)

// implService is the terminal JobService implementation. It composes the
// global job reader, the per-user state repo, the analytics snapshot, and
// the application-tracker writer — there is no further "next" repo layer.
type implService struct {
	jobs      JobReader
	userState UserJobStateRepository
	snapshot  SnapshotWriter
	appWriter ApplicationWriter
	ledger    SeenLedgerWriter
	cache     cache.Cache
	logger    implLogger
}

// NewImplService constructs the terminal JobService. cache and logger may
// be nil — nil cache disables caching entirely, nil logger no-ops.
func NewImplService(
	jobs JobReader,
	userState UserJobStateRepository,
	snapshot SnapshotWriter,
	appWriter ApplicationWriter,
	ledger SeenLedgerWriter,
	c cache.Cache,
	log implLogger,
) JobService {
	if log == nil {
		log = noopLogger{}
	}
	return &implService{
		jobs:      jobs,
		userState: userState,
		snapshot:  snapshot,
		appWriter: appWriter,
		ledger:    ledger,
		cache:     c,
		logger:    log,
	}
}

// pageCacheKey builds the global page cache key. Two callers with the
// same filter+cursor share the same cached page (per-user overlay is
// merged in after the cache lookup).
func pageCacheKey(cursor string, filter dto.JobFilter) string {
	h := sha256.New()
	_ = json.NewEncoder(h).Encode(filter)
	return fmt.Sprintf("%s%s:%s", feedPageCachePrefix, cursor, hex.EncodeToString(h.Sum(nil))[:16])
}

// userCountsKey is the per-user counts cache key.
func userCountsKey(userID string) string {
	return fmt.Sprintf("%s%s:counts", userCountsPrefix, userID)
}

// pageCacheEntry is the cached representation of one feed page. Holds the
// raw global jobs (no per-user overlay) plus the next-cursor value.
type pageCacheEntry struct {
	Jobs       []jobmodel.Job `json:"jobs"`
	NextCursor string         `json:"next_cursor"`
}

// List serves a feed page. Cache flow:
//  1. Try cache for (cursor + filter).
//  2. On miss, hit the DB and write back.
//  3. Always overlay the caller's user_job_state on top before returning.
//
// Non-"new" status tabs are routed to listByUserStatus which drives from
// user_job_state directly, bypassing the shared cache.
func (s *implService) List(ctx context.Context, filter dto.JobFilter, cursor string, limit int) (_ dto.PageResult, err error) {
	ctx, span := tracing.Start(ctx, "job.impl.List")
	defer tracing.End(span, &err)

	// Non-"new" status tabs drive from user_job_state — the shared page cache
	// is user-agnostic and in-memory status filtering produced sparse pages.
	if filter.Status != "" && filter.Status != jobcommon.JobStatusNew {
		return s.listByUserStatus(ctx, filter, cursor, limit)
	}

	userID := identity.UserIDFrom(ctx)

	// Strip status from the cache key — overlay is merged after the cache hit.
	pageFilter := filter
	pageFilter.Status = ""

	loader := func(ctx context.Context) (pageCacheEntry, error) {
		jobs, next, err := s.jobs.GetJobs(ctx, pageFilter, cursor, limit)
		if err != nil {
			return pageCacheEntry{}, apperr.ErrInternal.WithMessage("failed to fetch jobs")
		}
		return pageCacheEntry{Jobs: jobs, NextCursor: next}, nil
	}

	var entry pageCacheEntry
	if s.cache != nil {
		entry, err = cache.GetOrLoad(ctx, s.cache, pageCacheKey(cursor, pageFilter), feedPageCacheTTL, loader)
	} else {
		entry, err = loader(ctx)
	}
	if err != nil {
		var serr *apperr.ServiceError
		if stderrors.As(err, &serr) {
			return dto.PageResult{}, serr
		}
		return dto.PageResult{}, apperr.ErrInternal.WithMessage("failed to fetch jobs")
	}

	overlay := map[string]*jobmodel.UserJobState{}
	if userID != "" && len(entry.Jobs) > 0 {
		ids := make([]string, len(entry.Jobs))
		for i, j := range entry.Jobs {
			ids[i] = j.ID
		}
		overlay, err = s.userState.GetMany(ctx, userID, ids)
		if err != nil {
			s.logger.Warn("user_state overlay fetch failed", "err", err.Error())
			overlay = map[string]*jobmodel.UserJobState{}
		}
	}

	views := make([]dto.JobView, 0, len(entry.Jobs))
	for _, j := range entry.Jobs {
		st := overlay[j.ID]
		view := dto.JobView{Job: j, UserState: toUserStateView(st)}
		// Apply status filter after merge — cached page is user-agnostic.
		// "new" means "no workflow status" — saved is orthogonal and does
		// NOT eject the job from the new feed. A saved-only row therefore
		// appears in BOTH the new tab and the saved tab; bookmarking a job
		// is a pure marker that never moves it between workflow tabs.
		if filter.Status == jobcommon.JobStatusNew {
			if st != nil && st.Status != nil {
				continue
			}
		} else if filter.Status != "" && view.UserState.Status != filter.Status {
			continue
		}
		views = append(views, view)
	}
	return dto.PageResult{Items: views, NextCursor: entry.NextCursor}, nil
}

// listByUserStatus serves status tabs other than "new". Drives from
// user_job_state so every page is a full limit-sized result set — the
// previous in-memory status filter on the global feed produced sparse pages.
func (s *implService) listByUserStatus(ctx context.Context, filter dto.JobFilter, cursor string, limit int) (_ dto.PageResult, err error) {
	ctx, span := tracing.Start(ctx, "job.impl.listByUserStatus")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)
	// Pass the full filter so search/companies/date/experience filters apply
	// on status tabs, not just the "new" tab.
	jobs, next, err := s.userState.GetJobsByStatus(ctx, userID, filter, cursor, limit)
	if err != nil {
		return dto.PageResult{}, apperr.ErrInternal.WithMessage("failed to fetch jobs by status")
	}

	// Fetch full per-user state for each returned job so notes, timestamps,
	// and dismissal fields are populated — the same overlay used by the "new"
	// tab feed.
	overlay := map[string]*jobmodel.UserJobState{}
	if userID != "" && len(jobs) > 0 {
		ids := make([]string, len(jobs))
		for i, j := range jobs {
			ids[i] = j.ID
		}
		overlay, err = s.userState.GetMany(ctx, userID, ids)
		if err != nil {
			s.logger.Warn("user_state overlay fetch failed for status tab", "err", err.Error())
			overlay = map[string]*jobmodel.UserJobState{}
		}
	}

	views := make([]dto.JobView, len(jobs))
	for i, j := range jobs {
		views[i] = dto.JobView{
			Job:       j,
			UserState: toUserStateView(overlay[j.ID]),
		}
	}
	return dto.PageResult{Items: views, NextCursor: next}, nil
}

// Get returns a single job + per-user overlay. Bypasses the page cache —
// detail reads are infrequent enough that the simpler always-fresh path
// avoids consistency surprises right after a status update.
func (s *implService) Get(ctx context.Context, id string) (_ *dto.JobView, err error) {
	ctx, span := tracing.Start(ctx, "job.impl.Get")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)

	job, err := s.jobs.GetJobByID(ctx, id)
	if err != nil {
		if apperr.IsNotFound(err) {
			return nil, apperr.ErrNotFound.WithMessage("job not found")
		}
		return nil, apperr.ErrInternal.WithMessage("failed to fetch job")
	}
	view := dto.JobView{Job: *job, UserState: dto.UserJobStateView{Status: jobcommon.JobStatusNew}}
	if userID != "" {
		st, err := s.userState.Get(ctx, userID, id)
		if err == nil {
			view.UserState = toUserStateView(st)
		} else {
			var serr *apperr.ServiceError
			if !stderrors.As(err, &serr) || serr.Code != apperr.ErrNotFound.Code {
				s.logger.Warn("user_state get failed", "err", err.Error())
			}
		}
	}
	return &view, nil
}

// SetSaved toggles the orthogonal saved flag on user_job_state. It does
// NOT touch the workflow status column — a job can be saved while also
// applied/interviewing/etc. Idempotent at the repository layer.
func (s *implService) SetSaved(ctx context.Context, id string, saved bool) (err error) {
	ctx, span := tracing.Start(ctx, "job.impl.SetSaved")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)
	if userID == "" {
		return apperr.ErrInternal.WithMessage("user id required")
	}

	if err = s.userState.SetSaved(ctx, userID, id, saved); err != nil {
		return err
	}
	if saved {
		// Best-effort snapshot action update — analytics history is
		// orthogonal to per-user state. Failures here never block the save.
		if err = s.snapshot.UpdateSnapshotAction(ctx, id, jobmodel.StatusSaved); err != nil {
			s.logger.Warn("snapshot action update failed", "err", err.Error())
		}
	}
	s.invalidateUserCounts(ctx, userID)
	return nil
}

// SetStatus upserts the per-user workflow status (applied / interviewing /
// offer / rejected / dismissed). 'saved' is rejected — saved is orthogonal
// to workflow status and must go through SetSaved; the validation layer
// catches this earlier, but the impl guards as defence in depth.
func (s *implService) SetStatus(ctx context.Context, id, status string) (err error) {
	ctx, span := tracing.Start(ctx, "job.impl.SetStatus")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)
	if userID == "" {
		return apperr.ErrInternal.WithMessage("user id required")
	}
	if status == jobcommon.JobStatusSaved {
		return apperr.ErrBadRequest.WithMessage("use SetSaved for save/unsave")
	}

	if err = s.userState.Upsert(ctx, userID, id, dto.UpsertState{Status: status}); err != nil {
		return err
	}
	s.invalidateUserCounts(ctx, userID)

	// Side effects — non-fatal.
	switch status {
	case jobmodel.StatusApplied:
		if err = s.snapshot.UpdateSnapshotAction(ctx, id, jobmodel.StatusApplied); err != nil {
			s.logger.Warn("snapshot action update failed", "err", err.Error())
		}
		if err = s.appWriter.InsertAppliedApplication(ctx, id); err != nil {
			s.logger.Warn("applied application insert failed", "err", err.Error())
		}
	case jobmodel.StatusDismissed:
		if err = s.snapshot.UpdateSnapshotAction(ctx, id, jobmodel.StatusDismissed); err != nil {
			s.logger.Warn("snapshot action update failed", "err", err.Error())
		}
	}
	return nil
}

// SetNotes upserts the per-user notes column. Behaviour:
//
//   - Existing row with a workflow status: preserve the status, write notes.
//   - Existing row without a workflow status (only saved/notes): pass
//     JobStatusNew so Upsert leaves the status column NULL.
//   - No existing row: implicit save — pass JobStatusSaved so the new row
//     carries a saved_at and the non_empty CHECK constraint is satisfied.
func (s *implService) SetNotes(ctx context.Context, id, notes string) (err error) {
	ctx, span := tracing.Start(ctx, "job.impl.SetNotes")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)
	if userID == "" {
		return apperr.ErrInternal.WithMessage("user id required")
	}

	existing, _ := s.userState.Get(ctx, userID, id)
	var upsertStatus string
	switch {
	case existing == nil:
		// First touch — implicitly save so a saved_at anchors the row.
		upsertStatus = jobcommon.JobStatusSaved
	case existing.Status != nil:
		upsertStatus = *existing.Status
	default:
		// Row exists with NULL status (saved-only or notes-only). Use the
		// JobStatusNew sentinel so Upsert preserves the NULL status.
		upsertStatus = jobcommon.JobStatusNew
	}
	notesPtr := &notes
	if err = s.userState.Upsert(ctx, userID, id, dto.UpsertState{
		Status: upsertStatus,
		Notes:  notesPtr,
	}); err != nil {
		return err
	}
	// First note on an unseen job creates a saved row, which shifts the
	// counts ("new" → "saved"). Re-edits of an existing row leave status
	// alone, so we skip the invalidation there.
	if existing == nil {
		s.invalidateUserCounts(ctx, userID)
	}
	return nil
}

// Dismiss upserts the per-user row to 'dismissed' with reason+note, and
// (best-effort) updates the analytics snapshot + the global ledger so
// downstream pipelines stop reconsidering the job.
func (s *implService) Dismiss(ctx context.Context, id string, reason, note string) (err error) {
	ctx, span := tracing.Start(ctx, "job.impl.Dismiss")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)
	if userID == "" {
		return apperr.ErrInternal.WithMessage("user id required")
	}

	job, err := s.jobs.GetJobByID(ctx, id)
	if err != nil {
		if apperr.IsNotFound(err) {
			return apperr.ErrNotFound.WithMessage("job not found")
		}
		return apperr.ErrInternal.WithMessage("failed to fetch job")
	}

	reasonPtr := &reason
	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	if err = s.userState.Upsert(ctx, userID, id, dto.UpsertState{
		Status:          jobmodel.StatusDismissed,
		DismissalReason: reasonPtr,
		DismissalNote:   notePtr,
	}); err != nil {
		return err
	}
	s.invalidateUserCounts(ctx, userID)

	// Best-effort side effects.
	if err = s.ledger.UpdateSeenReason(ctx, job.JobPostID, jobcommon.SeenReasonUserDismissed); err != nil {
		s.logger.Warn("ledger seen reason update failed", "err", err.Error())
	}
	if err = s.snapshot.UpdateSnapshotAction(ctx, id, jobmodel.StatusDismissed); err != nil {
		s.logger.Warn("snapshot action update failed", "err", err.Error())
	}
	return nil
}

// BulkDismiss upserts every (user, job) pair in one round trip with the
// shared reason+note, then best-effort updates the ledger + snapshot.
func (s *implService) BulkDismiss(ctx context.Context, ids []string, reason, note string) (err error) {
	ctx, span := tracing.Start(ctx, "job.impl.BulkDismiss")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)
	if userID == "" {
		return apperr.ErrInternal.WithMessage("user id required")
	}
	if len(ids) == 0 {
		return nil
	}

	if err = s.userState.UpsertBulkDismiss(ctx, userID, ids, reason, note); err != nil {
		return err
	}
	s.invalidateUserCounts(ctx, userID)

	jobPostIDs, perr := s.jobs.GetJobPostIDs(ctx, ids)
	if perr == nil && len(jobPostIDs) > 0 {
		if err = s.ledger.UpdateSeenReasonBatch(ctx, jobPostIDs, jobcommon.SeenReasonUserDismissed); err != nil {
			s.logger.Warn("ledger seen reason batch update failed", "err", err.Error())
		}
	}
	if err = s.snapshot.UpdateSnapshotActionBatch(ctx, ids, jobmodel.StatusDismissed); err != nil {
		s.logger.Warn("snapshot action batch update failed", "err", err.Error())
	}
	return nil
}

// GetCounts returns the caller's per-user counts (saved / applied /
// dismissed). Cached for userCountsTTL; invalidated by every mutating
// method on this service.
func (s *implService) GetCompanies(ctx context.Context) ([]string, error) {
	return s.jobs.GetDistinctCompanyNames(ctx)
}

func (s *implService) GetCounts(ctx context.Context) (_ dto.Counts, err error) {
	ctx, span := tracing.Start(ctx, "job.impl.GetCounts")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)
	if userID == "" {
		return dto.Counts{}, apperr.ErrInternal.WithMessage("user id required")
	}

	loader := func(ctx context.Context) (dto.Counts, error) {
		counts, err := s.userState.CountsByUser(ctx, userID)
		if err != nil {
			return dto.Counts{}, err
		}
		// Derive the "new" count: total jobs minus the sum of workflow
		// statuses. Saved is orthogonal (a saved-only job still appears in
		// the new tab) so it is NOT subtracted here — otherwise a bookmark
		// would silently lower the new-tab badge. Non-fatal — if CountAll
		// fails, New stays 0.
		if total, terr := s.jobs.CountAll(ctx); terr == nil {
			sumNonNew := counts.Applied + counts.Dismissed +
				counts.Interviewing + counts.Offer + counts.Rejected
			if total > sumNonNew {
				counts.New = total - sumNonNew
			}
		}
		return counts, nil
	}
	if s.cache == nil {
		return loader(ctx)
	}
	return cache.GetOrLoad(ctx, s.cache, userCountsKey(userID), userCountsTTL, loader)
}

// invalidateUserCounts is a fire-and-forget cache delete. Failures are
// logged but never propagated — the next read pays the DB hit.
func (s *implService) invalidateUserCounts(ctx context.Context, userID string) {
	if s.cache == nil || userID == "" {
		return
	}
	if err := s.cache.Delete(ctx, userCountsKey(userID)); err != nil {
		s.logger.Warn("user counts cache invalidation failed", "user_id", userID, "err", err.Error())
	}
}

// toUserStateView projects a UserJobState row into the UI-facing read
// shape. A nil row, or a row with a NULL workflow status, maps to
// status="new". saved_at is orthogonal to the workflow status — a row
// with saved_at != NULL and status NULL still appears as "new" in the
// status field but populates SavedAt on the view.
func toUserStateView(s *jobmodel.UserJobState) dto.UserJobStateView {
	if s == nil {
		return dto.UserJobStateView{Status: jobcommon.JobStatusNew}
	}
	status := jobcommon.JobStatusNew
	if s.Status != nil {
		status = *s.Status
	}
	v := dto.UserJobStateView{
		Status:          status,
		Notes:           s.Notes,
		DismissalReason: s.DismissalReason,
		DismissalNote:   s.DismissalNote,
	}
	if s.SavedAt != nil {
		t := s.SavedAt.Format(time.RFC3339)
		v.SavedAt = &t
	}
	if s.AppliedAt != nil {
		t := s.AppliedAt.Format(time.RFC3339)
		v.AppliedAt = &t
	}
	if s.DismissedAt != nil {
		t := s.DismissedAt.Format(time.RFC3339)
		v.DismissedAt = &t
	}
	return v
}
