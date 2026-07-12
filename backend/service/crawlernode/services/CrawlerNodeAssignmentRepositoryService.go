package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/crawlernode/dto"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
	"gorm.io/gorm"
)

var _ AssignmentStore = (*assignmentRepositoryService)(nil)

// claimBatchSize bounds how many delay-eligible candidates ClaimNextForNode
// walks per poll before giving up for this cycle. Sized generously above the
// realistic number of distinct adapter types that could be simultaneously on
// cooldown or lose a claim race in one batch.
const claimBatchSize = 20

// queueCandidate is the narrow projection ClaimNextForNode uses to walk the
// claim batch before committing to a per-row UPDATE.
type queueCandidate struct {
	ID          string `gorm:"column:id"`
	CompanyID   string `gorm:"column:company_id"`
	AdapterType string `gorm:"column:adapter_type"`
}

// assignmentRepositoryService implements AssignmentStore against company_scan_queue
// using the locked_by column (migration 079) for node-specific ownership.
// Terminal — no next, no BaseNodeService.
type assignmentRepositoryService struct {
	db   *gorm.DB
	gate *AdapterDelayGate
}

// NewAssignmentRepositoryService constructs an AssignmentStore backed by
// Postgres. gate enforces the ats.inter_company_delay_s politeness cooldown
// across claims (see AdapterDelayGate.go) — never claim without consulting it.
func NewAssignmentRepositoryService(db *gorm.DB, gate *AdapterDelayGate) AssignmentStore {
	return &assignmentRepositoryService{db: db, gate: gate}
}

// ClaimNextForNode lists up to claimBatchSize queued, due candidates (ordered
// by priority DESC, queued_at ASC) and walks them in order: a candidate whose
// adapter type is still on its inter-company delay cooldown (AdapterDelayGate)
// is skipped without touching the DB; the first candidate that both clears
// the gate and wins a targeted per-row UPDATE (status='queued' -> 'running')
// is returned. Losing the UPDATE (0 rows affected — another claimant or
// ResetStuckForNodes got there first) is not fatal: the walk continues to the
// next candidate in the same batch rather than aborting the poll. Returns
// (nil, nil) when the queue is empty, every candidate is on cooldown, or
// every candidate lost its claim race — same "no work this poll" contract as
// before. When adapterType is empty, any adapter type is eligible (the
// crawler-node is generic and supports all adapters).
func (s *assignmentRepositoryService) ClaimNextForNode(ctx context.Context, nodeID, adapterType string) (_ *dto.ScanQueueItem, err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.ClaimNextForNode")
	defer tracing.End(span, &err)

	candidates, err := s.listClaimCandidates(ctx, adapterType)
	if err != nil {
		return nil, err
	}

	for _, c := range candidates {
		eligible, gateErr := s.gate.TryReserve(ctx, c.AdapterType)
		if gateErr != nil || !eligible {
			// On cooldown, or the delay-gate lookup failed (fail-safe,
			// non-fatal) — try the next candidate.
			continue
		}

		item, claimErr := s.claimQueueRow(ctx, c.ID, nodeID)
		if claimErr != nil {
			return nil, claimErr
		}
		if item == nil {
			// Lost the race to another claimant (or ResetStuckForNodes) —
			// move on to the next candidate rather than aborting the poll.
			continue
		}
		return item, nil
	}

	return nil, nil
}

// listClaimCandidates returns up to claimBatchSize queued, due rows ordered
// by priority DESC, queued_at ASC. No row lock is taken here — it is a
// read-only snapshot; safety against double-claiming comes from the targeted
// per-row UPDATE in claimQueueRow, not from locking this SELECT.
//
// The due-gate omits an explicit ::timestamptz cast on the '-infinity'
// literal — Postgres resolves the untyped literal's type from the other
// COALESCE argument (next_attempt_at) automatically, so the cast is
// redundant, and dropping it keeps the statement parseable by SQLite in
// unit tests (SQLite's parser rejects the `::type` cast operator outright).
func (s *assignmentRepositoryService) listClaimCandidates(ctx context.Context, adapterType string) (_ []queueCandidate, err error) {
	var candidates []queueCandidate
	var result *gorm.DB
	// INNER JOIN company_scan_settings so a company that was auto-paused or
	// deactivated after its row was queued is never claimed here — the
	// scheduler's PurgeInactive sweep is a best-effort cleanup, not the
	// authoritative gate. Every legitimate queue row has a matching settings
	// row (both Enqueue and EnqueueDue require one to insert in the first
	// place), so the INNER JOIN never spuriously excludes a valid candidate.
	if adapterType != "" {
		result = s.db.WithContext(ctx).Raw(`
			SELECT q.id, q.company_id, q.adapter_type
			FROM company_scan_queue q
			INNER JOIN company_scan_settings s ON s.company_id = q.company_id
			WHERE q.status = 'queued'
			  AND q.adapter_type = ?
			  AND COALESCE(q.next_attempt_at, '-infinity') <= NOW()
			  AND s.active = true
			  AND s.auto_paused = false
			ORDER BY q.priority DESC, q.queued_at ASC
			LIMIT ?
		`, adapterType, claimBatchSize).Scan(&candidates)
	} else {
		result = s.db.WithContext(ctx).Raw(`
			SELECT q.id, q.company_id, q.adapter_type
			FROM company_scan_queue q
			INNER JOIN company_scan_settings s ON s.company_id = q.company_id
			WHERE q.status = 'queued'
			  AND COALESCE(q.next_attempt_at, '-infinity') <= NOW()
			  AND s.active = true
			  AND s.auto_paused = false
			ORDER BY q.priority DESC, q.queued_at ASC
			LIMIT ?
		`, claimBatchSize).Scan(&candidates)
	}
	if result.Error != nil {
		return nil, errors.ErrInternal.WithMessage("list claim candidates: " + result.Error.Error())
	}
	return candidates, nil
}

// claimQueueRow attempts a targeted claim of a single queued row. Returns
// (nil, nil) — not an error — when the row is no longer status='queued'
// (lost the claim race between listClaimCandidates and this UPDATE).
func (s *assignmentRepositoryService) claimQueueRow(ctx context.Context, queueID, nodeID string) (_ *dto.ScanQueueItem, err error) {
	var item dto.ScanQueueItem
	result := s.db.WithContext(ctx).Raw(`
		UPDATE company_scan_queue
		SET status = 'running',
		    locked_by = ?,
		    started_at = NOW(),
		    heartbeat_at = NOW()
		WHERE id = ? AND status = 'queued'
		RETURNING id, company_id, adapter_type, status, source, started_at, heartbeat_at
	`, nodeID, queueID).Scan(&item)
	if result.Error != nil {
		return nil, errors.ErrInternal.WithMessage("claim next for node: " + result.Error.Error())
	}
	if item.ID == "" {
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

// FailForNode resets the queue row to status='queued', clears locked_by, and
// stamps next_attempt_at = NOW() + Backoff(consecutive_errors) so the row is
// NOT re-served instantly (the crawler retry hot-loop fix). The back-off is
// computed in SQL from company_scan_settings.consecutive_errors (already
// incremented on this same failure path by RecordScanResult), using the
// scancommon back-off constants bound as parameters so Go remains the single
// source of truth for base/cap/factor/max-shift. Returns ErrQueueRowMissing
// when locked_by != nodeID (ownership mismatch — never a silent no-op).
func (s *assignmentRepositoryService) FailForNode(ctx context.Context, queueID, nodeID string) (err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.FailForNode")
	defer tracing.End(span, &err)

	capSeconds := scancommon.BackoffCap.Seconds()
	baseSeconds := scancommon.BackoffBase.Seconds()

	result := s.db.WithContext(ctx).Exec(`
		UPDATE company_scan_queue q
		SET status = 'queued',
		    locked_by = NULL,
		    started_at = NULL,
		    heartbeat_at = NULL,
		    next_attempt_at = NOW() + LEAST(
		        ? * INTERVAL '1 second',
		        (? * INTERVAL '1 second')
		            * POWER(?, LEAST(?, COALESCE(s.consecutive_errors, 0)))
		    )
		FROM company_scan_settings s
		WHERE q.id = ?
		  AND q.locked_by = ?
		  AND s.company_id = q.company_id
	`, capSeconds, baseSeconds, scancommon.BackoffFactor, scancommon.BackoffMaxShift, queueID, nodeID)
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
