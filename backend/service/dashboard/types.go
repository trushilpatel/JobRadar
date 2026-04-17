package dashboard

// StatsView holds the data returned by GET /dashboard/stats.
type StatsView struct {
	JobsScannedToday int64         `json:"jobs_scanned_today"`
	JobsNew          int64         `json:"jobs_new"`
	JobsSaved        int64         `json:"jobs_saved"`
	JobsApplied      int64         `json:"jobs_applied"`
	PipelinePending  int64         `json:"pipeline_pending"`
	PipelineErrors   int64         `json:"pipeline_errors"`
	Funnel           FunnelSummary `json:"funnel"`
}

// FunnelSummary is a simplified funnel for the dashboard stats card.
type FunnelSummary struct {
	Scanned  int64 `json:"scanned"`
	Relevant int64 `json:"relevant"`
	Saved    int64 `json:"saved"`
	Applied  int64 `json:"applied"`
}

// ActivityView holds per-day scan activity. Reuses scan domain type.
type ActivityView struct {
	Date    string `json:"date"`
	Scanned int    `json:"scanned"`
	NewJobs int    `json:"new_jobs"`
}

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

// TechTrendRow represents a single technology's quarterly job-count data.
type TechTrendRow struct {
	Tech        string  `json:"tech"`
	Q1          int     `json:"q1"`           // job postings mentioning this tech, 3 quarters ago
	Q2          int     `json:"q2"`           // 2 quarters ago
	Q3          int     `json:"q3"`           // 1 quarter ago
	Q4          int     `json:"q4"`           // current quarter
	GrowthPct   float64 `json:"growth_pct"`   // for Rising: (q4-q3)/q3 * 100
	AdoptionPct float64 `json:"adoption_pct"` // q4 / total_jobs * 100
}

// TechTrends holds the full tech stack trend response.
type TechTrends struct {
	TotalJobs   int            `json:"total_jobs"`
	Rising      []TechTrendRow `json:"rising"`
	NewEntrants []TechTrendRow `json:"new_entrants"`
}
