package dto

// TechTrendRow represents a single technology's monthly job-count data.
type TechTrendRow struct {
	Tech        string  `json:"tech"`
	M1          int     `json:"m1"`           // job postings mentioning this tech, 3 months ago
	M2          int     `json:"m2"`           // 2 months ago
	M3          int     `json:"m3"`           // 1 month ago
	M4          int     `json:"m4"`           // current month
	GrowthPct   float64 `json:"growth_pct"`   // for Rising: (m4-m3)/m3 * 100; 0 when IsNew=true
	AdoptionPct float64 `json:"adoption_pct"` // m4 / total_jobs * 100
	IsNew       bool    `json:"is_new"`       // true when M3=0 (no prior-month baseline)
}

// TechTrends holds the full tech stack trend response.
type TechTrends struct {
	TotalJobs   int            `json:"total_jobs"`
	Rising      []TechTrendRow `json:"rising"`
	NewEntrants []TechTrendRow `json:"new_entrants"`
}
