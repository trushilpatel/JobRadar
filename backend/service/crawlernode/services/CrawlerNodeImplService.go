package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	crawlernodecommon "github.com/trushilpatel/jobradar/service/crawlernode/common"
	"github.com/trushilpatel/jobradar/service/crawlernode/dto"
	crawlernodemodel "github.com/trushilpatel/jobradar/service/crawlernode/models"
)

type implService struct {
	BaseNodeService
	nodeStore    NodeStore
	assignStore  AssignmentStore
	ledgerStore  LedgerStore
	scanLogStore ScanLogStore
	publisher    StreamPublisher
	scanSettings ScanSettingsReader
	tokenSecret  string
}

// NewImplService constructs the business-logic layer of the crawlernode decorator chain.
// tokenSecret must be non-empty — it is the HMAC-SHA256 key for token verification.
func NewImplService(
	nodeStore NodeStore,
	assignStore AssignmentStore,
	ledgerStore LedgerStore,
	scanLogStore ScanLogStore,
	publisher StreamPublisher,
	scanSettings ScanSettingsReader,
	tokenSecret string,
) NodeService {
	s := &implService{
		nodeStore:    nodeStore,
		assignStore:  assignStore,
		ledgerStore:  ledgerStore,
		scanLogStore: scanLogStore,
		publisher:    publisher,
		scanSettings: scanSettings,
		tokenSecret:  tokenSecret,
	}
	// BaseNodeService is not used for delegation here since implService does not
	// wrap another NodeService — it is the terminal implementation.
	// We set a no-op base to avoid the nil-next panic.
	s.BaseNodeService = BaseNodeService{next: s}
	return s
}

// GetWork implements the orchestrator's work-assignment endpoint.
func (s *implService) GetWork(ctx context.Context, nodeID, adapterType string, holdingIDs []string) (_ *dto.WorkResponse, err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.impl.GetWork")
	defer tracing.End(span, &err)

	// 1. Extend heartbeats for in-flight assignments.
	var acked, missed []string
	if len(holdingIDs) > 0 {
		acked, missed, err = s.assignStore.ExtendHeartbeats(ctx, nodeID, holdingIDs)
		if err != nil {
			return nil, fmt.Errorf("GetWork: extend heartbeats: %w", err)
		}
	}

	// 2. Claim the next work item.
	queueItem, err := s.assignStore.ClaimNextForNode(ctx, nodeID, adapterType)
	if err != nil {
		return nil, fmt.Errorf("GetWork: claim next: %w", err)
	}

	resp := &dto.WorkResponse{
		HeartbeatAcked:  acked,
		HeartbeatMissed: missed,
	}
	if resp.HeartbeatAcked == nil {
		resp.HeartbeatAcked = []string{}
	}
	if resp.HeartbeatMissed == nil {
		resp.HeartbeatMissed = []string{}
	}

	if queueItem == nil {
		return resp, nil // 204 path — no work available
	}

	// 3. Get adapter config.
	adapterConfig, err := s.scanSettings.GetAdapterConfig(ctx, queueItem.CompanyID)
	if err != nil {
		// If scan settings are unavailable, drain the queue entry to avoid a stuck row.
		_ = s.assignStore.CompleteForNode(ctx, queueItem.ID, nodeID)
		return resp, nil
	}

	// 4. Get known job IDs for dedup optimisation.
	knownIDs, err := s.ledgerStore.GetActiveJobIDsByCompany(ctx, queueItem.CompanyID)
	if err != nil {
		// Non-fatal degradation — proceed with empty known IDs.
		knownIDs = []string{}
	}

	resp.Assignment = &dto.WorkUnit{
		AssignmentID:  queueItem.ID,
		CompanyID:     queueItem.CompanyID,
		AdapterType:   queueItem.AdapterType,
		AdapterConfig: adapterConfig,
		KnownIDs:      knownIDs,
		Source:        queueItem.Source,
	}
	return resp, nil
}

// SubmitResults processes a batch of crawled jobs from a crawler node.
func (s *implService) SubmitResults(ctx context.Context, nodeID string, req dto.ResultsRequest) (_ *dto.ResultsResponse, err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.impl.SubmitResults")
	defer tracing.End(span, &err)

	resp := &dto.ResultsResponse{}
	published := 0
	newJobs := 0

	// 1. Partition jobs: non-USA gate.
	var toPublish []dto.CrawledJob
	var nonUSAIDs []string
	for _, job := range req.Jobs {
		if isNonUSA(job) {
			nonUSAIDs = append(nonUSAIDs, job.ID)
		} else {
			toPublish = append(toPublish, job)
		}
	}

	// 2. Mark non-USA jobs in ledger (reason=non_usa) — must come before publish.
	if len(nonUSAIDs) > 0 {
		if err := s.ledgerStore.MarkSeenBatch(ctx, nonUSAIDs, req.CompanyID, crawlernodecommon.ScanReasonNonUSA); err != nil {
			return nil, fmt.Errorf("SubmitResults: mark non-usa: %w", err)
		}
	}

	// 3. Find new jobs (not in ledger already).
	if len(toPublish) > 0 {
		jobIDs := make([]string, len(toPublish))
		for i, j := range toPublish {
			jobIDs[i] = j.ID
		}

		seen, err := s.ledgerStore.HasSeenBatch(ctx, jobIDs)
		if err != nil {
			return nil, fmt.Errorf("SubmitResults: has seen batch: %w", err)
		}

		var newJobList []dto.CrawledJob
		var newJobIDs []string
		for _, job := range toPublish {
			if !seen[job.ID] {
				newJobList = append(newJobList, job)
				newJobIDs = append(newJobIDs, job.ID)
			}
		}

		if len(newJobIDs) > 0 {
			// 4. Ledger BEFORE publish — critical invariant.
			if err := s.ledgerStore.MarkSeenBatch(ctx, newJobIDs, req.CompanyID, crawlernodecommon.ScanReasonFetch); err != nil {
				return nil, fmt.Errorf("SubmitResults: mark seen: %w", err)
			}

			// 5. Publish to NATS per-job.
			for _, job := range newJobList {
				payload, pubErr := marshalJob(job, req.CompanyID, req.AdapterType)
				if pubErr != nil {
					continue // skip bad serialisation — per-job isolation
				}
				subject := "jobs.raw." + req.AdapterType
				if pubErr := s.publisher.PublishWithMsgID(ctx, subject, payload, job.ID); pubErr != nil {
					continue // publish failure — ledger row stays; re-scan picks it up
				}
				published++
			}
			newJobs = len(newJobIDs)
		}
	}

	// 6. Removed-job detection.
	if len(req.PreviousKnownIDs) > 0 {
		currentSet := make(map[string]struct{}, len(req.Jobs))
		for _, j := range req.Jobs {
			currentSet[j.ID] = struct{}{}
		}
		var removedIDs []string
		for _, id := range req.PreviousKnownIDs {
			if _, ok := currentSet[id]; !ok {
				removedIDs = append(removedIDs, id)
			}
		}
		if len(removedIDs) > 0 {
			if err := s.ledgerStore.MarkRemovedBatch(ctx, removedIDs); err != nil {
				// Non-fatal — removed-job detection is best-effort observability.
				_ = err
			}
			resp.Removed = len(removedIDs)
		}
	}

	resp.Published = published
	resp.New = newJobs

	// 7. On final, complete the queue row and record the scan log.
	if req.Final {
		completeErr := s.assignStore.CompleteForNode(ctx, req.AssignmentID, nodeID)
		if completeErr != nil && !stderrors.Is(completeErr, domainerrs.ErrQueueRowMissing) {
			return nil, fmt.Errorf("SubmitResults: complete for node: %w", completeErr)
		}
		resp.Completed = true

		scanResult := dto.ScanResult{
			FetchedCount: req.FetchedCount,
			NewCount:     newJobs,
			DurationMs:   req.DurationMs,
		}
		if logErr := s.scanLogStore.RecordScanResult(ctx, req.CompanyID, scanResult); logErr != nil {
			// Non-fatal — scan log failure must never block the primary completion.
			_ = logErr
		}
	}

	return resp, nil
}

// ReportError handles an error reported by a crawler node for an assignment.
func (s *implService) ReportError(ctx context.Context, nodeID string, req dto.ErrorRequest) (err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.impl.ReportError")
	defer tracing.End(span, &err)

	// 1. Reset the queue row (re-queue for retry).
	failErr := s.assignStore.FailForNode(ctx, req.AssignmentID, nodeID)
	if failErr != nil && !stderrors.Is(failErr, domainerrs.ErrQueueRowMissing) {
		return fmt.Errorf("ReportError: fail for node: %w", failErr)
	}
	// ErrQueueRowMissing is non-fatal — row was already reset or re-claimed.

	// 2. Always record the scan result — even when FailForNode had ErrQueueRowMissing.
	msg := req.ErrorMessage
	scanResult := dto.ScanResult{
		DurationMs: req.DurationMs,
		ErrMsg:     msg,
	}
	if logErr := s.scanLogStore.RecordScanResult(ctx, req.CompanyID, scanResult); logErr != nil {
		_ = logErr // non-fatal
	}

	return nil
}

// ProvisionNode creates a new crawler node record and returns its plaintext token once.
func (s *implService) ProvisionNode(ctx context.Context, name string) (_ *dto.CreateNodeResponse, err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.impl.ProvisionNode")
	defer tracing.End(span, &err)

	// Generate a UUID v4 token using crypto/rand.
	tokenUUID, err := generateUUID()
	if err != nil {
		return nil, fmt.Errorf("ProvisionNode: generate token: %w", err)
	}
	tokenStr := tokenUUID.String()

	// Compute HMAC-SHA256(token, secret).
	tokenHash := computeHMAC(tokenStr, s.tokenSecret)

	prefix := tokenStr[:crawlernodecommon.TokenPrefixLen]
	node := crawlernodemodel.CrawlerNode{
		Name:        name,
		TokenPrefix: prefix,
		TokenHash:   &tokenHash,
		Status:      crawlernodecommon.NodeStatusActive,
	}

	created, err := s.nodeStore.Create(ctx, node)
	if err != nil {
		return nil, err // already wrapped with ErrConflict or ErrInternal
	}

	now := time.Now()
	return &dto.CreateNodeResponse{
		NodeView: dto.NodeView{
			ID:        created.ID,
			Name:      created.Name,
			Status:    created.Status,
			Online:    false, // just provisioned — never seen yet
			CreatedAt: now,
		},
		Token: tokenStr, // returned exactly once; never stored
	}, nil
}

// ListNodes returns all crawler nodes, optionally filtered by status.
func (s *implService) ListNodes(ctx context.Context, status string) (_ []dto.NodeView, err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.impl.ListNodes")
	defer tracing.End(span, &err)

	nodes, err := s.nodeStore.List(ctx, status)
	if err != nil {
		return nil, err
	}

	views := make([]dto.NodeView, 0, len(nodes))
	for _, n := range nodes {
		online := n.LastSeenAt != nil && time.Since(*n.LastSeenAt) <= crawlernodecommon.OnlineThreshold
		views = append(views, dto.NodeView{
			ID:         n.ID,
			Name:       n.Name,
			Status:     n.Status,
			Online:     online,
			LastSeenAt: n.LastSeenAt,
			CreatedAt:  n.CreatedAt,
		})
	}
	return views, nil
}

// SetNodeStatus updates the status of a crawler node.
// Transitions from 'retired' (terminal state) are blocked.
func (s *implService) SetNodeStatus(ctx context.Context, nodeID, status string) (err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.impl.SetNodeStatus")
	defer tracing.End(span, &err)

	if status == crawlernodecommon.NodeStatusRetired {
		// Use RetireNode for retirement — it clears token_hash atomically.
		return s.RetireNode(ctx, nodeID)
	}

	// Guard against retired → active/disabled transitions.
	// We use SetStatus with a WHERE clause that excludes retired rows.
	// If the row is retired, RowsAffected = 0 → ErrNotFound from the store.
	return s.nodeStore.SetStatus(ctx, nodeID, status)
}

// RetireNode sets the node to 'retired' and atomically clears its token_hash.
func (s *implService) RetireNode(ctx context.Context, nodeID string) (err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.impl.RetireNode")
	defer tracing.End(span, &err)

	return s.nodeStore.Retire(ctx, nodeID)
}

// isNonUSA returns true if a job's location is definitively non-USA:
// LocationKnown must be true AND all locations must be non-USA.
// If LocationKnown=false (thin adapters, Workday, Meta), the job passes through.
func isNonUSA(job dto.CrawledJob) bool {
	if !job.LocationKnown {
		return false
	}
	if len(job.AllLocations) == 0 {
		return false
	}
	for _, loc := range job.AllLocations {
		if isUSA(loc) {
			return false
		}
	}
	return true
}

// isUSA returns true if a location string is considered US-based.
func isUSA(loc string) bool {
	// Match common US location patterns. This mirrors the existing engine's logic.
	for _, marker := range []string{"US", "USA", "United States", "Remote"} {
		if loc == marker {
			return true
		}
	}
	return false
}

// generateUUID returns a new UUID v4 using crypto/rand.
func generateUUID() (uuid.UUID, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return uuid.UUID{}, fmt.Errorf("uuid generate: %w", err)
	}
	// Set version 4 and variant bits.
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return uuid.UUID(id), nil
}

// computeHMAC returns hex(HMAC-SHA256(message, key)).
func computeHMAC(message, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// rawJobWire mirrors apps/crawler/ats/types.RawJob JSON field names.
// Defined here to avoid importing an apps/ package from a service/ package.
type rawJobWire struct {
	ID            string           `json:"id"`
	CompanyID     string           `json:"company_id"`
	Adapter       string           `json:"adapter"`
	Title         string           `json:"title"`
	Location      string           `json:"location"`
	URL           string           `json:"url"`
	EmployType    string           `json:"employ_type"`
	FullContent   *fullContentWire `json:"full_content,omitempty"`
	LocationKnown bool             `json:"location_known,omitempty"`
	AllLocations  []string         `json:"all_locations,omitempty"`
}

type fullContentWire struct {
	Description  string `json:"description"`
	Requirements string `json:"requirements"`
	Compensation string `json:"compensation"`
	Department   string `json:"department"`
}

// marshalJob serialises a CrawledJob to the jobs.raw.<adapter> NATS wire format.
func marshalJob(job dto.CrawledJob, companyID, adapterType string) ([]byte, error) {
	wire := rawJobWire{
		ID:            job.ID,
		CompanyID:     companyID,
		Adapter:       adapterType,
		Title:         job.Title,
		Location:      job.Location,
		URL:           job.URL,
		EmployType:    job.EmploymentType,
		LocationKnown: job.LocationKnown,
		AllLocations:  job.AllLocations,
	}
	if job.FullContent != nil {
		wire.FullContent = &fullContentWire{
			Description:  job.FullContent.Description,
			Requirements: job.FullContent.Requirements,
			Compensation: job.FullContent.Salary,
			Department:   job.FullContent.Department,
		}
	}
	return jsonMarshal(wire)
}

// jsonMarshal is a thin wrapper over encoding/json so tests can verify the payload shape.
func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}
