package model

// AdapterType
const (
    AdapterGreenhouse = "greenhouse"
    AdapterLever      = "lever"
    AdapterAshby      = "ashby"
    AdapterGoogle     = "google"
)

// ScanStatus
const (
    ScanStatusOK      = "ok"
    ScanStatusError   = "error"
    ScanStatusPending = "pending"
)

// SeenReason
const (
    SeenReasonFetch         = "fetch"
    SeenReasonLLMIrrelevant = "llm_irrelevant"
    SeenReasonUserDismissed = "user_dismissed"
    SeenReasonError         = "error"
)

// EmployType
const (
    EmployFullTime   = "full_time"
    EmployPartTime   = "part_time"
    EmployContract   = "contract"
    EmployInternship = "internship"
)

// JobStatus
const (
    JobStatusNew       = "new"
    JobStatusSaved     = "saved"
    JobStatusApplied   = "applied"
    JobStatusDismissed = "dismissed"
)

// ApplicationStage
const (
    StageApplied     = "applied"
    StagePhoneScreen = "phone_screen"
    StageTechnical   = "technical"
    StageFinalRound  = "final_round"
    StageOffer       = "offer"
    StageRejected    = "rejected"
)

// DiscardReason
const (
    DiscardReasonDismissed   = "user_dismissed"
    DiscardReasonNotRelevant = "user_not_relevant"
)

// QueueStatus — values for company_scan_queue.status
const (
    QueueStatusQueued     = "queued"
    QueueStatusProcessing = "processing"
)

// QueueSource — who enqueued the company
const (
    QueueSourceScheduler = "scheduler"
    QueueSourceManual    = "manual"
)
