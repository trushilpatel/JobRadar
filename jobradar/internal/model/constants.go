package model

// AdapterType
const (
    AdapterGreenhouse = "greenhouse"
    AdapterLever      = "lever"
    AdapterAshby      = "ashby"
    AdapterWorkday    = "workday"
    AdapterWorkable   = "workable"
    AdapterChromedp   = "chromedp"
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
    JobStatusNew     = "new"
    JobStatusSaved   = "saved"
    JobStatusApplied = "applied"
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
