package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/crawlernode/dto"
	"gorm.io/gorm"
)

// assignmentRepositoryService implements AssignmentStore against company_scan_queue
// using the locked_by column (migration 079) for node-specific ownership.
// Terminal — no next, no BaseNodeService.
type assignmentRepositoryService struct {
	db *gorm.DB
}

// NewAssignmentRepositoryService constructs an AssignmentStore backed by Postgres.
func NewAssignmentRepositoryService(db *gorm.DB) AssignmentStore {
	return &assignmentRepositoryService{db: db}
}

// ClaimNextForNode claims the next queued row for the given adapterType using
// FOR UPDATE SKIP LOCKED and sets locked_by = nodeID atomically.
func (s *assignmentRepositoryService) ClaimNextForNode(ctx context.Context, nodeID, adapterType string) (_ *dto.ScanQueueItem, err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.ClaimNextForNode")
	defer tracing.End(span, &err)

	var item dto.ScanQueueItem
	result := s.db.WithContext(ctx).Raw(`
		UPDATE company_scan_queue
		SET status = 'running',
		    locked_by = ?,
		    started_at = NOW(),
		    heartbeat_at = NOW()
		WHERE id = (
		    SELECT id FROM company_scan_queue
		    WHERE status = 'queued'
		      AND adapter_type = ?
		    ORDER BY priority DESC, queued_at ASC
		    LIMIT 1
		    FOR UPDATE SKIP LOCKED
		)
		RETURNING id, company_id, adapter_type, status, source, started_at, heartbeat_at
	`, nodeID, adapterType).Scan(&item)

	if result.Error != nil {
		return nil, errors.ErrInternal.WithMessage("claim next for node: " + result.Error.Error())
	}
	if item.ID == "" {
		// No eligible row — queue empty or all rows SKIP LOCKED by other claimants.
		return nil, nil
	}
	return &item, nil
}

// ExtendHeartbeats refreshes heartbeat_at for each queueID where locked_by = nodeID.
// Returns IDs successfully updated in acked; IDs where locked_by no longer matches in missed.
func (s *assignmentRepositoryService) ExtendHeartbeats(ctx context.Context, nodeID string, queueIDs []string) (acked, missed []string, err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.ExtendHeartbeats")
	defer tracing.End(span, &err)

	if len(queueIDs) == 0 {
		return []string{}, []string{}, nil
	}

	// Update rows where locked_by = nodeID and collect the IDs updated.
	var updatedIDs []string
	result := s.db.WithContext(ctx).Raw(`
		UPDATE company_scan_queue
		SET heartbeat_at = NOW()
		WHERE id IN ?
		  AND locked_by = ?
		RETURNING id
	`, queueIDs, nodeID).Scan(&updatedIDs)

	if result.Error != nil {
		return nil, nil, errors.ErrInternal.WithMessage("extend heartbeats: " + result.Error.Error())
	}

	// Build set for O(1) lookup.
	ackedSet := make(map[string]struct{}, len(updatedIDs))
	for _, id := range updatedIDs {
		ackedSet[id] = struct{}{}
	}

	acked = make([]string, 0, len(updatedIDs))
	missed = make([]string, 0)
	for _, id := range queueIDs {
		if _, ok := ackedSet[id]; ok {
			acked = append(acked, id)
		} else {
			missed = append(missed, id)
		}
	}
	return acked, missed, nil
}

// CompleteForNode deletes the queue row. Returns ErrQueueRowMissing when
// locked_by != nodeID (ownership mismatch — never silently no-op).
func (s *assignmentRepositoryService) CompleteForNode(ctx context.Context, queueID, nodeID string) (err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.CompleteForNode")
	defer tracing.End(span, &err)

	result := s.db.WithContext(ctx).
		Exec("DELETE FROM company_scan_queue WHERE id = ? AND locked_by = ?", queueID, nodeID)
	if result.Error != nil {
		return errors.ErrInternal.WithMessage("complete for node: " + result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return errors.ErrQueueRowMissing.WithMessage("CompleteForNode: no queue row for id=" + queueID + " locked_by=" + nodeID)
	}
	return nil
}

// FailForNode resets the queue row to status='queued' and clears locked_by.
// Returns ErrQueueRowMissing when locked_by != nodeID.
func (s *assignmentRepositoryService) FailForNode(ctx context.Context, queueID, nodeID string) (err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.FailForNode")
	defer tracing.End(span, &err)

	result := s.db.WithContext(ctx).Exec(`
		UPDATE company_scan_queue
		SET status = 'queued',
		    locked_by = NULL,
		    started_at = NULL,
		    heartbeat_at = NULL
		WHERE id = ?
		  AND locked_by = ?
	`, queueID, nodeID)
	if result.Error != nil {
		return errors.ErrInternal.WithMessage("fail for node: " + result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return errors.ErrQueueRowMissing.WithMessage("FailForNode: no queue row for id=" + queueID + " locked_by=" + nodeID)
	}
	return nil
}

// ResetStuckForNodes reclaims running rows claimed by any node whose
// heartbeat is older than staleDur. Returns the count of rows reset.
func (s *assignmentRepositoryService) ResetStuckForNodes(ctx context.Context, staleDur time.Duration) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.ResetStuckForNodes")
	defer tracing.End(span, &err)

	result := s.db.WithContext(ctx).Exec(`
		UPDATE company_scan_queue
		SET status = 'queued',
		    locked_by = NULL,
		    started_at = NULL,
		    heartbeat_at = NULL
		WHERE locked_by IS NOT NULL
		  AND status = 'running'
		  AND COALESCE(heartbeat_at, started_at) < NOW() - ? * INTERVAL '1 second'
	`, staleDur.Seconds())
	if result.Error != nil {
		return 0, errors.ErrInternal.WithMessage("reset stuck for nodes: " + result.Error.Error())
	}
	return int(result.RowsAffected), nil
}
