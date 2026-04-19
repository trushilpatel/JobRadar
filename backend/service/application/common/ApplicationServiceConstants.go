package common

// ApplicationStage values for job_application_tracker.stage
const (
	StageApplied     = "applied"
	StagePhoneScreen = "phone_screen"
	StageTechnical   = "technical"
	StageFinalRound  = "final_round"
	StageOffer       = "offer"
	StageRejected    = "rejected"
)

// DisplayStage UI-facing stage groupings
const (
	DisplayStageWaiting      = "Waiting"
	DisplayStageInterviewing = "Interviewing"
	DisplayStageOffer        = "Offer received"
)
