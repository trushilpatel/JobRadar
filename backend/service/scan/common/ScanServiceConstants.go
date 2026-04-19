package common

// ScanStatus values for companies.last_scan_status
const (
	ScanStatusOK      = "ok"
	ScanStatusError   = "error"
	ScanStatusPending = "pending"
)

// QueueStatus values for company_scan_queue.status
const (
	QueueStatusQueued     = "queued"
	QueueStatusProcessing = "processing"
)

// QueueSource values for company_scan_queue.source
const (
	QueueSourceScheduler = "scheduler"
	QueueSourceManual    = "manual"
)
