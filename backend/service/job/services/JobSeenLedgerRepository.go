package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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

func (s *seenLedgerStore) UpdateSeenReasonBatch(ctx context.Context, jobIDs []string, reason jobcommon.SeenReason) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateSeenReasonBatch")
	defer tracing.End(span, &err)
	if len(jobIDs) == 0 {
		return nil
	}
	res := s.db.WithContext(ctx).Model(&jobmodel.JobSeenLedger{}).
		Where("job_id IN ?", jobIDs).
		Update("reason", string(reason))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected < int64(len(jobIDs)) {
		return domainerrs.ErrLedgerRowMissing.WithMessage(
			fmt.Sprintf("UpdateSeenReasonBatch: %d of %d ledger rows missing (reason=%s)",
				int64(len(jobIDs))-res.RowsAffected, len(jobIDs), reason))
	}
	return nil
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

	res := s.db.WithContext(ctx).Exec(query, args...)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected < int64(len(entries)) {
		return domainerrs.ErrLedgerRowMissing.WithMessage(
			fmt.Sprintf("UpdateRetryBatch: %d of %d ledger rows missing",
				int64(len(entries))-res.RowsAffected, len(entries)))
	}
	return nil
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

	res := s.db.WithContext(ctx).Exec(query, args...)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected < int64(len(entries)) {
		return domainerrs.ErrLedgerRowMissing.WithMessage(
			fmt.Sprintf("UpdateRetryBatchWithPayload: %d of %d ledger rows missing",
				int64(len(entries))-res.RowsAffected, len(entries)))
	}
	return nil
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
