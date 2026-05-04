package dto

// MonthlyJobPoint holds new/removed counts for one calendar month.
type MonthlyJobPoint struct {
	Month   string `json:"month"`
	New     int    `json:"new"`
	Removed int    `json:"removed"`
}

// CompanyVelocity holds this week vs last week new job counts.
type CompanyVelocity struct {
	ThisWeek int `json:"this_week"`
	LastWeek int `json:"last_week"`
}

// CompanyAnalytics is the response envelope for GET /companies/:id/analytics.
type CompanyAnalytics struct {
	Monthly  []MonthlyJobPoint `json:"monthly"`
	Velocity CompanyVelocity   `json:"velocity"`
}
