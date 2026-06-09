package common

import "time"

// Node status values — match crawler_nodes.status CHECK constraint.
const (
	NodeStatusActive   = "active"
	NodeStatusDisabled = "disabled"
	NodeStatusRetired  = "retired"
)

// ValidNodeStatuses contains all allowed status values.
var ValidNodeStatuses = []string{NodeStatusActive, NodeStatusDisabled, NodeStatusRetired}

// NodeStatusTerminal is the status from which no transitions are allowed.
const NodeStatusTerminal = NodeStatusRetired

// TokenPrefixLen is the number of leading characters of the plaintext token
// stored as token_prefix for fast index lookup.
const TokenPrefixLen = 8

// HeartbeatStaleDuration is the default threshold after which a running
// assignment with no heartbeat update is considered stuck and eligible for
// reset by ResetStuckForNodes.
const HeartbeatStaleDuration = 5 * time.Minute

// OnlineThreshold is the window within which a node is considered online.
// If last_seen_at is older than this, the node is shown as offline in the admin UI.
const OnlineThreshold = 90 * time.Second

// MaxNodeNameLen is the maximum length of a crawler node name.
const MaxNodeNameLen = 255

// MaxErrorMessageLen is the maximum length of an error message in ErrorRequest.
const MaxErrorMessageLen = 2000

// MaxJobsPerBatch is the maximum number of jobs allowed in a single ResultsRequest.
const MaxJobsPerBatch = 5000

// ScanReasonFetch is the ledger reason for a freshly fetched job.
const ScanReasonFetch = "fetch"

// ScanReasonNonUSA is the ledger reason when a job is dropped for non-USA location.
const ScanReasonNonUSA = "non_usa"

// ScanStatusOK is the status written to company_scan_log on success.
const ScanStatusOK = "ok"

// ScanStatusError is the status written to company_scan_log on error.
const ScanStatusError = "error"
