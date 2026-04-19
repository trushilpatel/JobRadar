package dto

// InterviewRow represents a single active application in the interviewing tracker.
type InterviewRow struct {
	ApplicationID int64  `json:"application_id"`
	JobID         string `json:"job_id"`
	CompanyName   string `json:"company_name"`
	JobTitle      string `json:"job_title"`
	Stage         string `json:"stage"`         // raw DB stage
	DisplayStage  string `json:"display_stage"` // "Waiting" | "Interviewing" | "Offer received"
	StageAt       string `json:"stage_at"`      // ISO timestamp
}
