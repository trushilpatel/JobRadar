package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	commondb "github.com/trushilpatel/jobradar/common/db"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ SeenLedgerStore = (*seenLedgerStore)(nil)

// RetryLedgerEntry describes a per-job retry state update destined for
// job_pipeline_ledger. Used by UpdateRetryBatch to write distinct
// retry_count and next_attempt_at values for many rows in a single call.
type RetryLedgerEntry struct {
	JobID         string
	RetryCount    int
	NextAttemptAt time.Time
}

// RetryLedgerEntryWithPayload is a per-job retry state update that also
// persists the serialized RawJob payload and the adapter name to the ledger.
// Used by UpdateRetryBatchWithPayload so the sweeper can re-publish the
// original job payload without needing to re-fetch it from the source.
type RetryLedgerEntryWithPayload struct {
	JobID         string
	Adapter       string
	Payload       json.RawMessage
	RetryCount    int
	NextAttemptAt time.Time
}

// RetryJobRecord is returned by FetchDueRetryJobs. It carries everything
// the sweeper needs to re-publish the job without touching the original
// NATS message.
type RetryJobRecord struct {
	JobID      string
	Adapter    string
	Payload    json.RawMessage
	RetryCount int
}

// seenLedgerStore is the GORM-backed terminal repo for the
// job_pipeline_ledger table. It implements the SeenLedgerStore composite
// interface declared in JobService.go.
type seenLedgerStore struct {
	db *gorm.DB
}

// NewSeenLedgerStore constructs the ledger store.
func NewSeenLedgerStore(db *gorm.DB) SeenLedgerStore {
	return &seenLedgerStore{db: db}
}

func (s *seenLedgerStore) HasSeen(ctx context.Context, jobID string) (_ bool, err error) {
	ctx, span := tracing.Start(ctx, "job.store.HasSeen")
	defer tracing.End(span, &err)
	var count int64
	err = s.db.WithContext(ctx).Model(&jobmodel.JobSeenLedger{}).
		Where("job_id = ?", jobID).
		Count(&count).Error
	return count > 0, err
}

func (s *seenLedgerStore) HasSeenBatch(ctx context.Context, jobIDs []string) (_ map[string]bool, err error) {
	ctx, span := tracing.Start(ctx, "job.store.HasSeenBatch")
	defer tracing.End(span, &err)
	if len(jobIDs) == 0 {
		return map[string]bool{}, nil
	}
	var found []string
	err = s.db.WithContext(ctx).Model(&jobmodel.JobSeenLedger{}).
		Where("job_id IN ?", jobIDs).
		Pluck("job_id", &found).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(jobIDs))
	for _, id := range found {
		result[id] = true
	}
	return result, nil
}

// CountByReason returns the current number of job_pipeline_ledger rows at
// the given reason with removed_at IS NULL — e.g. the sweeper's
// reason='retry' backlog size.
func (s *seenLedgerStore) CountByReason(ctx context.Context, reason jobcommon.SeenReason) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "job.ledger.store.CountByReason")
	defer tracing.End(span, &err)
	var count int64
	err = s.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM job_pipeline_ledger WHERE reason = ? AND removed_at IS NULL
	`, string(reason)).Scan(&count).Error
	return int(count), err
}

func (s *seenLedgerStore) MarkSeen(ctx context.Context, jobID, companyID, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.MarkSeen")
	defer tracing.End(span, &err)
	row := jobmodel.JobSeenLedger{
		JobID:     jobID,
		CompanyID: companyID,
		Reason:    reason,
	}
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *seenLedgerStore) MarkSeenBatch(ctx context.Context, jobIDs []string, companyID, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.MarkSeenBatch")
	defer tracing.End(span, &err)
	if len(jobIDs) == 0 {
		return nil
	}
	rows := make([]jobmodel.JobSeenLedger, len(jobIDs))
	for i, id := range jobIDs {
		rows[i] = jobmodel.JobSeenLedger{JobID: id, CompanyID: companyID, Reason: reason}
	}
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(rows, 500).Error
}

func (s *seenLedgerStore) UpdateSeenReason(ctx context.Context, jobID string, reason jobcommon.SeenReason) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateSeenReason")
	defer tracing.End(span, &err)
	res := s.db.WithContext(ctx).Model(&jobmodel.JobSeenLedger{}).
		Where("job_id = ?", jobID).
		Update("reason", string(reason))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domainerrs.ErrLedgerRowMissing.WithMessage(
			fmt.Sprintf("UpdateSeenReason: no ledger row for job_id=%s (reason=%s)", jobID, reason))
	}
	return nil
}

// execBatchWithRetry runs run (an idempotent GORM UPDATE/Exec producing a
// *gorm.DB result) inside db.WithRetry so a transient Supavisor connection drop
// is retried, then evaluates the RowsAffected sentinel ONCE on the final
// successful exec. run is retried only on the connection-drop class — the
// ErrLedgerRowMissing decision is made outside the retried closure so a
// short-count (a business outcome) is never retried. context is the message
// prefix for the sentinel error. Returns ErrLedgerRowMissing when fewer than
// expected rows matched, the raw error on a non-recoverable exec failure, or
// nil on a full match.
func execBatchWithRetry(ctx context.Context, sentinelContext string, expected int64, run func() *gorm.DB) error {
	var rowsAffected int64
	if err := commondb.WithRetry(ctx, func() error {
		res := run()
		if res.Error != nil {
			return res.Error
		}
		rowsAffected = res.RowsAffected
		return nil
	}); err != nil {
		return err
	}
	if rowsAffected < expected {
		return domainerrs.ErrLedgerRowMissing.WithMessage(
			fmt.Sprintf("%s: %d of %d ledger rows missing",
				sentinelContext, expected-rowsAffected, expected))
	}
	return nil
}

func (s *seenLedgerStore) UpdateSeenReasonBatch(ctx context.Context, jobIDs []string, reason jobcommon.SeenReason) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateSeenReasonBatch")
	defer tracing.End(span, &err)
	if len(jobIDs) == 0 {
		return nil
	}
	// Idempotent: re-running the same reason UPDATE is a no-op after the first
	// success (the target reason is already set). Retried only on a conn drop;
	// the short-count sentinel is evaluated after success (never retried).
	return execBatchWithRetry(ctx, "UpdateSeenReasonBatch", int64(len(jobIDs)), func() *gorm.DB {
		return s.db.WithContext(ctx).Model(&jobmodel.JobSeenLedger{}).
			Where("job_id IN ?", jobIDs).
			Update("reason", string(reason))
	})
}

func (s *seenLedgerStore) UpdateRetryBatch(ctx context.Context, entries []RetryLedgerEntry) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateRetryBatch")
	defer tracing.End(span, &err)
	if len(entries) == 0 {
		return nil
	}

	valuesClauses := make([]string, 0, len(entries))
	args := make([]any, 0, len(entries)*3)
	for i, e := range entries {
		base := i * 3
		valuesClauses = append(valuesClauses, fmt.Sprintf("($%d, $%d::int, $%d::timestamptz)", base+1, base+2, base+3))
		args = append(args, e.JobID, e.RetryCount, e.NextAttemptAt)
	}

	query := "UPDATE job_pipeline_ledger AS l " +
		"SET reason = '" + string(jobcommon.SeenReasonRetry) + "', retry_count = v.retry_count, next_attempt_at = v.next_attempt_at " +
		"FROM (VALUES " + strings.Join(valuesClauses, ",") + ") " +
		"AS v(job_id, retry_count, next_attempt_at) " +
		"WHERE l.job_id = v.job_id"

	// Idempotent: the VALUES-join UPDATE writes the same retry_count /
	// next_attempt_at each run. Retry only the raw Exec (conn-drop surface);
	// evaluate the row-count sentinel once, after success.
	return execBatchWithRetry(ctx, "UpdateRetryBatch", int64(len(entries)), func() *gorm.DB {
		return s.db.WithContext(ctx).Exec(query, args...)
	})
}

func (s *seenLedgerStore) UpdateRetryBatchWithPayload(ctx context.Context, entries []RetryLedgerEntryWithPayload) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateRetryBatchWithPayload")
	defer tracing.End(span, &err)
	if len(entries) == 0 {
		return nil
	}

	valuesClauses := make([]string, 0, len(entries))
	args := make([]any, 0, len(entries)*5)
	for i, e := range entries {
		base := i * 5
		valuesClauses = append(valuesClauses, fmt.Sprintf(
			"($%d, $%d::int, $%d::timestamptz, $%d::jsonb, $%d)",
			base+1, base+2, base+3, base+4, base+5,
		))
		args = append(args, e.JobID, e.RetryCount, e.NextAttemptAt, []byte(e.Payload), e.Adapter)
	}

	query := "UPDATE job_pipeline_ledger AS l " +
		"SET reason = '" + string(jobcommon.SeenReasonRetry) + "', retry_count = v.retry_count, next_attempt_at = v.next_attempt_at, payload = v.payload, adapter = v.adapter " +
		"FROM (VALUES " + strings.Join(valuesClauses, ",") + ") " +
		"AS v(job_id, retry_count, next_attempt_at, payload, adapter) " +
		"WHERE l.job_id = v.job_id"

	// Idempotent: the VALUES-join UPDATE writes the same retry_count /
	// next_attempt_at / payload / adapter each run. Retry only the raw Exec
	// (conn-drop surface); evaluate the row-count sentinel once, after success.
	return execBatchWithRetry(ctx, "UpdateRetryBatchWithPayload", int64(len(entries)), func() *gorm.DB {
		return s.db.WithContext(ctx).Exec(query, args...)
	})
}

func (s *seenLedgerStore) FetchDueRetryJobs(ctx context.Context, limit int) (_ []RetryJobRecord, err error) {
	ctx, span := tracing.Start(ctx, "job.store.FetchDueRetryJobs")
	defer tracing.End(span, &err)
	var rows []RetryJobRecord
	res := s.db.WithContext(ctx).Raw(`
		SELECT job_id, COALESCE(adapter, '') AS adapter, payload, retry_count
		FROM job_pipeline_ledger
		WHERE reason = ? AND next_attempt_at <= CURRENT_TIMESTAMP
		ORDER BY next_attempt_at ASC
		LIMIT ?
	`, string(jobcommon.SeenReasonRetry), limit).Scan(&rows)
	if res.Error != nil {
		return nil, res.Error
	}
	return rows, nil
}

// FetchDueRetryJobsWithPayload returns up to limit ledger rows where
// reason='retry', next_attempt_at <= now(), payload IS NOT NULL, adapter IS
// NOT NULL, and adapter != ”. This excludes legacy rows written before payload
// persistence was added (AC-3) so the sweeper never tries to publish a nil
// payload.
func (s *seenLedgerStore) FetchDueRetryJobsWithPayload(ctx context.Context, limit int) (_ []RetryJobRecord, err error) {
	ctx, span := tracing.Start(ctx, "job.store.FetchDueRetryJobsWithPayload")
	defer tracing.End(span, &err)
	var rows []RetryJobRecord
	res := s.db.WithContext(ctx).Raw(`
		SELECT job_id, COALESCE(adapter, '') AS adapter, payload, retry_count
		FROM job_pipeline_ledger
		WHERE reason = ? AND next_attempt_at <= CURRENT_TIMESTAMP
		  AND payload IS NOT NULL
		  AND adapter IS NOT NULL AND adapter != ''
		ORDER BY next_attempt_at ASC
		LIMIT ?
	`, string(jobcommon.SeenReasonRetry), limit).Scan(&rows)
	if res.Error != nil {
		return nil, res.Error
	}
	return rows, nil
}

func (s *seenLedgerStore) ResetRetryToFetch(ctx context.Context, jobID string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.ResetRetryToFetch")
	defer tracing.End(span, &err)
	res := s.db.WithContext(ctx).Exec(`
		UPDATE job_pipeline_ledger
		SET reason = ?, payload = NULL, adapter = NULL, next_attempt_at = NULL
		WHERE job_id = ?
	`, string(jobcommon.SeenReasonFetch), jobID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domainerrs.ErrLedgerRowMissing.WithMessage(
			fmt.Sprintf("ResetRetryToFetch: no ledger row for job_id=%s", jobID))
	}
	return nil
}

func (s *seenLedgerStore) EscalateRetryToError(ctx context.Context, jobID string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.EscalateRetryToError")
	defer tracing.End(span, &err)
	res := s.db.WithContext(ctx).Exec(`
		UPDATE job_pipeline_ledger
		SET reason = ?
		WHERE job_id = ? AND reason = ?
	`, string(jobcommon.SeenReasonError), jobID, string(jobcommon.SeenReasonRetry))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domainerrs.ErrLedgerRowMissing.WithMessage(
			fmt.Sprintf("EscalateRetryToError: no retry ledger row for job_id=%s", jobID))
	}
	return nil
}

// EscalateStuckRetryJobs transitions up to limit ledger rows from
// reason='retry' to reason='error' when their next_attempt_at is older than
// now() - stuckThreshold. This recovers jobs that were never re-published by
// the sweeper (e.g. sustained NATS outage). Returns the number of rows updated.
func (s *seenLedgerStore) EscalateStuckRetryJobs(ctx context.Context, stuckThreshold time.Duration, limit int) (_ int64, err error) {
	ctx, span := tracing.Start(ctx, "job.store.EscalateStuckRetryJobs")
	defer tracing.End(span, &err)
	res := s.db.WithContext(ctx).Exec(`
		UPDATE job_pipeline_ledger
		SET reason = ?
		WHERE job_id IN (
			SELECT job_id FROM job_pipeline_ledger
			WHERE reason = ? AND next_attempt_at < NOW() - (? * INTERVAL '1 second')
			LIMIT ?
		)
	`, string(jobcommon.SeenReasonError), string(jobcommon.SeenReasonRetry),
		int64(stuckThreshold.Seconds()), limit)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// ReclaimStuckFetchRows DELETEs up to limit ledger rows that are stranded at
// reason='fetch' with removed_at IS NULL and first_seen_at older than now() -
// threshold. These are jobs the crawler marked seen before publishing but the
// filter never transitioned to a terminal verdict (e.g. a NATS message lost
// before consumption). Because HasSeenBatch ignores reason and removed_at, the
// only way to clear the dedup block is to remove the row — escalating to
// reason='error' would keep the job deduped forever. Once deleted, the next
// scheduled crawl re-discovers the still-live job and re-drives it.
//
// This is a count-based sweep, not a targeted write: 0 deleted rows is a valid
// steady-state outcome, so it returns RowsAffected and does NOT return
// ErrLedgerRowMissing (unlike the targeted UPDATE methods above).
func (s *seenLedgerStore) ReclaimStuckFetchRows(ctx context.Context, threshold time.Duration, limit int) (_ int64, err error) {
	ctx, span := tracing.Start(ctx, "job.store.ReclaimStuckFetchRows")
	defer tracing.End(span, &err)
	res := s.db.WithContext(ctx).Exec(`
		DELETE FROM job_pipeline_ledger
		WHERE job_id IN (
			SELECT job_id FROM job_pipeline_ledger
			WHERE reason = ? AND removed_at IS NULL
			  AND first_seen_at < NOW() - (? * INTERVAL '1 second')
			LIMIT ?
		)
	`, string(jobcommon.SeenReasonFetch), int64(threshold.Seconds()), limit)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func (s *seenLedgerStore) MarkRemovedBatch(ctx context.Context, jobIDs []string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.MarkRemovedBatch")
	defer tracing.End(span, &err)
	if len(jobIDs) == 0 {
		return nil
	}
	now := time.Now()
	return s.db.WithContext(ctx).Model(&jobmodel.JobSeenLedger{}).
		Where("job_id IN ?", jobIDs).
		Where("removed_at IS NULL").
		Update("removed_at", now).Error
}

func (s *seenLedgerStore) GetActiveJobIDsByCompany(ctx context.Context, companyID string) (ids []string, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetActiveJobIDsByCompany")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Model(&jobmodel.JobSeenLedger{}).
		Where("company_id = ? AND removed_at IS NULL", companyID).
		Pluck("job_id", &ids).Error
	return ids, err
}

// -----------------------------------------------------------------------------
// LedgerService — narrow façade for crawler workers (HasSeenBatch /
// MarkSeenBatch / GetActiveJobIDsByCompany / MarkRemovedBatch).
// -----------------------------------------------------------------------------

var _ LedgerService = (*ledgerService)(nil)

type ledgerService struct {
	store SeenLedgerStore
}

// NewLedgerService constructs the ledger façade workers consume.
func NewLedgerService(s SeenLedgerStore) LedgerService {
	return &ledgerService{store: s}
}

func (l *ledgerService) HasSeenBatch(ctx context.Context, jobIDs []string) (map[string]bool, error) {
	return l.store.HasSeenBatch(ctx, jobIDs)
}

func (l *ledgerService) MarkSeenBatch(ctx context.Context, jobIDs []string, companyID, reason string) error {
	return l.store.MarkSeenBatch(ctx, jobIDs, companyID, reason)
}

func (l *ledgerService) GetActiveJobIDsByCompany(ctx context.Context, companyID string) ([]string, error) {
	return l.store.GetActiveJobIDsByCompany(ctx, companyID)
}

func (l *ledgerService) MarkRemovedBatch(ctx context.Context, jobIDs []string) error {
	return l.store.MarkRemovedBatch(ctx, jobIDs)
}
