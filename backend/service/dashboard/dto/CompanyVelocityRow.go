package dto

// CompanyVelocityRow holds per-company 4-week velocity data.
type CompanyVelocityRow struct {
	CompanyID   string  `json:"company_id"`
	CompanyName string  `json:"company_name"`
	WeekCurrent int     `json:"week_current"`
	Week1       int     `json:"week_1"` // 1 week ago
	Week2       int     `json:"week_2"` // 2 weeks ago
	Week3       int     `json:"week_3"` // 3 weeks ago
	Multiplier  float64 `json:"multiplier"`
	VelocityTag string  `json:"velocity_tag"` // "spike" | "rising" | "normal"
}
