package dto

// TechTrendRow represents a single technology's quarterly job-count data.
type TechTrendRow struct {
	Tech        string  `json:"tech"`
	Q1          int     `json:"q1"`           // job postings mentioning this tech, 3 quarters ago
	Q2          int     `json:"q2"`           // 2 quarters ago
	Q3          int     `json:"q3"`           // 1 quarter ago
	Q4          int     `json:"q4"`           // current quarter
	GrowthPct   float64 `json:"growth_pct"`   // for Rising: (q4-q3)/q3 * 100; 0 when IsNew=true
	AdoptionPct float64 `json:"adoption_pct"` // q4 / total_jobs * 100
	IsNew       bool    `json:"is_new"`       // true when Q3=0 (no prior-quarter baseline)
}

// TechTrends holds the full tech stack trend response.
type TechTrends struct {
	TotalJobs   int            `json:"total_jobs"`
	Rising      []TechTrendRow `json:"rising"`
	NewEntrants []TechTrendRow `json:"new_entrants"`
}
