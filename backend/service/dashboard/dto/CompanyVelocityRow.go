package dto

// CompanyVelocityRow holds per-company 4-week velocity data.
type CompanyVelocityRow struct {
	CompanyID   string  `json:"company_id"`
	CompanyName string  `json:"company_name"`
	WeekCurrent int     `gorm:"column:week_current" json:"week_current"`
	Week1       int     `gorm:"column:week_1"       json:"week_1"` // 1 week ago
	Week2       int     `gorm:"column:week_2"       json:"week_2"` // 2 weeks ago
	Week3       int     `gorm:"column:week_3"       json:"week_3"` // 3 weeks ago
	Multiplier  float64 `json:"multiplier"`
	VelocityTag string  `json:"velocity_tag"` // "spike" | "rising" | "normal"
}
