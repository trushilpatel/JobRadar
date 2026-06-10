package services

import (
	"context"
	"time"

	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
)

// JobService is the single user-facing contract for the job domain. Every
// method pulls the caller's user ID from ctx (via identity.UserIDFrom) so
// that callers cannot accidentally broaden a query to all users.
//
// PageResult uses keyset pagination (cursor on added_at,id) — Total is not
// computed; per-user counts are served by GetCounts (cached separately).
type JobService interface {
	List(ctx context.Context, filter dto.JobFilter, cursor string, limit int) (dto.PageResult, error)
	Get(ctx context.Context, id string) (*dto.JobView, error)
	SetSaved(ctx context.Context, id string, saved bool) error
	SetStatus(ctx context.Context, id string, status string) error
	SetNotes(ctx context.Context, id string, notes string) error
	Dismiss(ctx context.Context, id string, reason, note string) error
	BulkDismiss(ctx context.Context, ids []string, reason, note string) error
	GetCounts(ctx context.Context) (dto.Counts, error)
	GetCompanies(ctx context.Context) ([]string, error)
}

// BaseService auto-delegates every JobService method to the next layer.
// Embed this in ImplService, ValidationService, and CacheService; override
// only the methods that layer is responsible for.
type BaseService struct {
	next JobService
}

// NewBaseService constructs a BaseService wrapping next. Panics if next is
// nil — a nil next is always a wiring error, not a runtime condition.
func NewBaseService(next JobService) BaseService {
	if next == nil {
		panic("BaseService: next must not be nil — check wire setup")
	}
	return BaseService{next: next}
}

func (b *BaseService) List(ctx context.Context, filter dto.JobFilter, cursor string, limit int) (dto.PageResult, error) {
	return b.next.List(ctx, filter, cursor, limit)
}

func (b *BaseService) Get(ctx context.Context, id string) (*dto.JobView, error) {
	return b.next.Get(ctx, id)
}

func (b *BaseService) SetSaved(ctx context.Context, id string, saved bool) error {
	return b.next.SetSaved(ctx, id, saved)
}

func (b *BaseService) SetStatus(ctx context.Context, id, status string) error {
	return b.next.SetStatus(ctx, id, status)
}

func (b *BaseService) SetNotes(ctx context.Context, id, notes string) error {
	return b.next.SetNotes(ctx, id, notes)
}

func (b *BaseService) Dismiss(ctx context.Context, id string, reason, note string) error {
	return b.next.Dismiss(ctx, id, reason, note)
}

func (b *BaseService) BulkDismiss(ctx context.Context, ids []string, reason, note string) error {
	return b.next.BulkDismiss(ctx, ids, reason, note)
}

func (b *BaseService) GetCounts(ctx context.Context) (dto.Counts, error) {
	return b.next.GetCounts(ctx)
}

func (b *BaseService) GetCompanies(ctx context.Context) ([]string, error) {
	return b.next.GetCompanies(ctx)
}

// -----------------------------------------------------------------------------
// Worker- and store-facing interfaces. CLAUDE.md requires every interface
// in a slice be declared here; the concrete implementations live in their
// own files (e.g. JobRepositoryService.go, UserJobStateRepositoryService.go).
// -----------------------------------------------------------------------------

// LedgerService is the dedup + removal contract crawler workers depend on.
type LedgerService interface {
	HasSeenBatch(ctx context.Context, jobIDs []string) (map[string]bool, error)
	MarkSeenBatch(ctx context.Context, jobIDs []string, companyID, reason string) error
	GetActiveJobIDsByCompany(ctx context.Context, companyID string) ([]string, error)
	MarkRemovedBatch(ctx context.Context, jobIDs []string) error
}

// PipelineService is the analyzer-worker contract for writing fully-enriched
// jobs into the global jobs table + analytics snapshot + ledger.
type PipelineService interface {
	SaveEnrichedJob(ctx context.Context, input dto.EnrichedJobInput) error
	MarkLedgerError(ctx context.Context, jobID string) error
	MarkLedgerIrrelevant(ctx context.Context, jobID string) error
	MarkLedgerNonUSA(ctx context.Context, jobID string) error
	HasSeenJob(ctx context.Context, jobID string) (bool, error)
}

// JobReader is the global (non-per-user) read contract used by dashboards
// and worker code that needs raw rows from the jobs table. List uses
// keyset pagination; the cursor is opaque to callers.
type JobReader interface {
	GetJobs(ctx context.Context, filter dto.JobFilter, cursor string, limit int) ([]jobmodel.Job, string, error)
	GetJobByID(ctx context.Context, id string) (*jobmodel.Job, error)
	GetJobsByIDs(ctx context.Context, ids []string) ([]jobmodel.Job, error)
	GetJobsAddedByDay(ctx context.Context, days int) ([]dto.DailyJobCount, error)
	GetJobPostIDs(ctx context.Context, ids []string) ([]string, error)
	CountAll(ctx context.Context) (int64, error)
	GetDistinctCompanyNames(ctx context.Context) ([]string, error)
}

// JobWriter is the global (non-per-user) write contract used by the
// analyzer worker.
type JobWriter interface {
	InsertJob(ctx context.Context, job *jobmodel.Job) error
	DeleteJob(ctx context.Context, id string) error
}

// JobStore composes JobReader and JobWriter for callers that need both.
type JobStore interface {
	JobReader
	JobWriter
}

// UserJobStateRepository is the per-user state contract. All user-scoped
// methods take userID explicitly — empty userID is rejected with
// ErrInternal.WithMessage("user id required") so a forgotten ctx scoping
// never silently broadens a query to all users.
type UserJobStateRepository interface {
	Get(ctx context.Context, userID, jobID string) (*jobmodel.UserJobState, error)
	GetMany(ctx context.Context, userID string, jobIDs []string) (map[string]*jobmodel.UserJobState, error)
	// GetJobsByStatus returns up to limit jobs for userID with the given status
	// and any additional filter criteria (search, companies, date range, etc.).
	// Ordered by (jobs.added_at DESC, jobs.id DESC). Cursor format matches GetJobs.
	// Drives from user_job_state so every page is a full limit-sized result set.
	GetJobsByStatus(ctx context.Context, userID string, filter dto.JobFilter, cursor string, limit int) ([]jobmodel.Job, string, error)
	Upsert(ctx context.Context, userID, jobID string, state dto.UpsertState) error
	// SetSaved is the orthogonal save/unsave path. Idempotent. Never touches
	// the workflow status column; saved_at IS NOT NULL is the source of truth.
	SetSaved(ctx context.Context, userID, jobID string, saved bool) error
	UpsertBulkDismiss(ctx context.Context, userID string, jobIDs []string, reason, note string) error
	Delete(ctx context.Context, userID, jobID string) error
	CountsByUser(ctx context.Context, userID string) (dto.Counts, error)
}

// SeenLedgerReader checks if a job has already been processed.
type SeenLedgerReader interface {
	HasSeen(ctx context.Context, jobID string) (bool, error)
	HasSeenBatch(ctx context.Context, jobIDs []string) (map[string]bool, error)
}

// SeenLedgerWriter marks jobs as seen / removed / retried.
type SeenLedgerWriter interface {
	MarkSeen(ctx context.Context, jobID, companyID, reason string) error
	MarkSeenBatch(ctx context.Context, jobIDs []string, companyID, reason string) error
	UpdateSeenReason(ctx context.Context, jobID string, reason jobcommon.SeenReason) error
	UpdateSeenReasonBatch(ctx context.Context, jobIDs []string, reason jobcommon.SeenReason) error
	UpdateRetryBatch(ctx context.Context, entries []RetryLedgerEntry) error
	// UpdateRetryBatchWithPayload sets reason='retry' with payload and adapter
	// columns alongside retry_count and next_attempt_at. Used by the sweeper path
	// (LLM exhaustion) where the RawJob payload must be preserved for re-publish.
	UpdateRetryBatchWithPayload(ctx context.Context, entries []RetryLedgerEntryWithPayload) error
	// FetchDueRetryJobs returns up to limit ledger rows where reason='retry' and
	// next_attempt_at <= now(), ordered by next_attempt_at ASC.
	FetchDueRetryJobs(ctx context.Context, limit int) ([]RetryJobRecord, error)
	// ResetRetryToFetch transitions a single row to reason='fetch', clearing
	// payload, adapter, and next_attempt_at. ErrLedgerRowMissing on 0 rows.
	ResetRetryToFetch(ctx context.Context, jobID string) error
	// EscalateRetryToError transitions a row from reason='retry' to reason='error'.
	// ErrLedgerRowMissing when no row matched (missing row or wrong reason).
	EscalateRetryToError(ctx context.Context, jobID string) error
	MarkRemovedBatch(ctx context.Context, jobIDs []string) error
}

// SeenLedgerCompanyReader provides company-scoped reads on the ledger.
type SeenLedgerCompanyReader interface {
	GetActiveJobIDsByCompany(ctx context.Context, companyID string) ([]string, error)
}

// SeenLedgerStore composes all ledger interfaces.
type SeenLedgerStore interface {
	SeenLedgerReader
	SeenLedgerWriter
	SeenLedgerCompanyReader
}

// SnapshotWriter is the analyzer / impl-service contract for writing to
// the job_analytics table.
type SnapshotWriter interface {
	InsertSnapshot(ctx context.Context, input dto.SnapshotInput) error
	UpdateSnapshotAction(ctx context.Context, id, action string) error
	UpdateSnapshotActionBatch(ctx context.Context, ids []string, action string) error
}

// TechTagWriter writes normalized technology tags for a job into job_tech_tags.
type TechTagWriter interface {
	InsertTechTags(ctx context.Context, jobID, companyID string, techs []string, taggedAt time.Time) error
}

// ApplicationWriter is the narrow write contract used by the job impl
// service to auto-insert application tracker rows when a user advances a
// job to applied. user_id is sourced from ctx by the application service.
type ApplicationWriter interface {
	InsertAppliedApplication(ctx context.Context, jobResultID string) error
}

// implLogger is the minimal logging surface implService needs. Tests pass a
// noop; production wires the platform logger.
type implLogger interface {
	Warn(msg string, kv ...any)
}

// noopLogger swallows every log call. Used as a default so wire setup
// doesn't have to thread a logger through every test seam.
type noopLogger struct{}

func (noopLogger) Warn(string, ...any) {}
