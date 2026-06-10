package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/service/crawlernode/dto"
	crawlernodemodel "github.com/trushilpatel/jobradar/service/crawlernode/models"
)

// NodeStore manages the crawler_nodes table (CRUD + auth lookup).
// Terminal — no next, no BaseNodeService.
type NodeStore interface {
	Create(ctx context.Context, node crawlernodemodel.CrawlerNode) (*crawlernodemodel.CrawlerNode, error)
	GetByTokenPrefix(ctx context.Context, prefix string) ([]*crawlernodemodel.CrawlerNode, error)
	UpdateLastSeen(ctx context.Context, nodeID string) error
	SetStatus(ctx context.Context, nodeID, status string) error
	List(ctx context.Context, status string) ([]crawlernodemodel.CrawlerNode, error)
	// Retire sets status='retired' and clears token_hash atomically.
	Retire(ctx context.Context, nodeID string) error
}

// AssignmentStore manages the company_scan_queue in node-claiming mode.
// Terminal — no next, no BaseNodeService.
type AssignmentStore interface {
	// ClaimNextForNode claims the next queued row for the given adapterType using
	// FOR UPDATE SKIP LOCKED and sets locked_by = nodeID.
	// Returns nil, nil when no eligible row exists.
	ClaimNextForNode(ctx context.Context, nodeID, adapterType string) (*dto.ScanQueueItem, error)

	// ExtendHeartbeats updates heartbeat_at for each queueID where locked_by = nodeID.
	// acked contains IDs successfully updated; missed contains IDs where locked_by
	// no longer matches (reclaimed by ResetStuckForNodes) — the node must abort those.
	ExtendHeartbeats(ctx context.Context, nodeID string, queueIDs []string) (acked, missed []string, err error)

	// CompleteForNode deletes the queue row. Returns ErrQueueRowMissing when
	// locked_by != nodeID (ownership mismatch — never silently no-op).
	CompleteForNode(ctx context.Context, queueID, nodeID string) error

	// FailForNode resets the queue row to status='queued' and clears locked_by.
	// Returns ErrQueueRowMissing when locked_by != nodeID.
	FailForNode(ctx context.Context, queueID, nodeID string) error

	// ResetStuckForNodes reclaims running rows claimed by any node whose
	// heartbeat is older than staleDur. Returns count of rows reset.
	ResetStuckForNodes(ctx context.Context, staleDur time.Duration) (int, error)
}

// LedgerStore is the narrow job_pipeline_ledger façade used by the orchestrator.
// Terminal — no next, no BaseNodeService.
type LedgerStore interface {
	// HasSeenBatch returns a map of jobID → true if already in the ledger.
	// Returns empty map (not error) for an empty jobIDs slice.
	HasSeenBatch(ctx context.Context, jobIDs []string) (map[string]bool, error)
	// MarkSeenBatch inserts ledger rows for new jobIDs. ON CONFLICT DO NOTHING.
	// Must be called BEFORE PublishWithMsgID (ledger-before-publish invariant).
	MarkSeenBatch(ctx context.Context, jobIDs []string, companyID, reason string) error
	// MarkRemovedBatch marks jobs as removed in the ledger.
	MarkRemovedBatch(ctx context.Context, jobIDs []string) error
	// GetActiveJobIDsByCompany returns all non-removed job IDs for a company.
	GetActiveJobIDsByCompany(ctx context.Context, companyID string) ([]string, error)
}

// ScanLogStore writes scan outcomes to company_scan_log.
// Terminal — no next, no BaseNodeService.
type ScanLogStore interface {
	RecordScanResult(ctx context.Context, companyID string, r dto.ScanResult) error
}

// StreamPublisher publishes a message to a NATS JetStream subject with dedup.
type StreamPublisher interface {
	PublishWithMsgID(ctx context.Context, subject string, payload []byte, msgID string) error
}

// ScanSettingsReader reads adapter_config and related settings per company.
// Provided by the existing scan service.
type ScanSettingsReader interface {
	GetAdapterConfig(ctx context.Context, companyID string) (*string, error)
}

// NodeService is the single contract for the entire crawlernode decorator chain.
type NodeService interface {
	// node-facing endpoints
	GetWork(ctx context.Context, nodeID, adapterType string, holdingIDs []string) (*dto.WorkResponse, error)
	SubmitResults(ctx context.Context, nodeID string, req dto.ResultsRequest) (*dto.ResultsResponse, error)
	ReportError(ctx context.Context, nodeID string, req dto.ErrorRequest) error

	// admin-facing endpoints
	ProvisionNode(ctx context.Context, name string) (*dto.CreateNodeResponse, error)
	ListNodes(ctx context.Context, status string) ([]dto.NodeView, error)
	SetNodeStatus(ctx context.Context, nodeID, status string) error
	RetireNode(ctx context.Context, nodeID string) error
}

// BaseNodeService auto-delegates every NodeService method to next.
// Embed this in validation / impl decorators and override only what you own.
type BaseNodeService struct{ next NodeService }

// NewBaseNodeService constructs a BaseNodeService wrapping next.
// Panics if next is nil to surface wiring mistakes at startup.
func NewBaseNodeService(next NodeService) BaseNodeService {
	if next == nil {
		panic("BaseNodeService: next must not be nil — check wire setup")
	}
	return BaseNodeService{next: next}
}

func (b *BaseNodeService) GetWork(ctx context.Context, nodeID, adapterType string, holdingIDs []string) (*dto.WorkResponse, error) {
	return b.next.GetWork(ctx, nodeID, adapterType, holdingIDs)
}

func (b *BaseNodeService) SubmitResults(ctx context.Context, nodeID string, req dto.ResultsRequest) (*dto.ResultsResponse, error) {
	return b.next.SubmitResults(ctx, nodeID, req)
}

func (b *BaseNodeService) ReportError(ctx context.Context, nodeID string, req dto.ErrorRequest) error {
	return b.next.ReportError(ctx, nodeID, req)
}

func (b *BaseNodeService) ProvisionNode(ctx context.Context, name string) (*dto.CreateNodeResponse, error) {
	return b.next.ProvisionNode(ctx, name)
}

func (b *BaseNodeService) ListNodes(ctx context.Context, status string) ([]dto.NodeView, error) {
	return b.next.ListNodes(ctx, status)
}

func (b *BaseNodeService) SetNodeStatus(ctx context.Context, nodeID, status string) error {
	return b.next.SetNodeStatus(ctx, nodeID, status)
}

func (b *BaseNodeService) RetireNode(ctx context.Context, nodeID string) error {
	return b.next.RetireNode(ctx, nodeID)
}
