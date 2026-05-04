package dto

// CompanyVelocity holds per-company 4-week velocity data for the insights panel.
type CompanyVelocity struct {
	CompanyID   string  `json:"company_id"`
	CompanyName string  `json:"company_name"`
	WeekCurrent int     `gorm:"column:week_current" json:"week_current"`
	Week1       int     `gorm:"column:week_1"       json:"week_1"`
	Week2       int     `gorm:"column:week_2"       json:"week_2"`
	Week3       int     `gorm:"column:week_3"       json:"week_3"`
	Multiplier  float64 `json:"multiplier"`
	VelocityTag string  `json:"velocity_tag"` // "spike" | "rising" | "normal"
}
