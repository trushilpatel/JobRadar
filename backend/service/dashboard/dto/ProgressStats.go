package dto

// ProgressStats holds aggregated user progress stats for the My Progress panel.
type ProgressStats struct {
	AppliedCount         int64 `json:"applied_count"`
	AwaitingReply        int64 `json:"awaiting_reply"`
	InterviewingCount    int64 `json:"interviewing_count"`
	InterviewingThisWeek int64 `json:"interviewing_this_week"`
	OfferCount           int64 `json:"offer_count"`
}
