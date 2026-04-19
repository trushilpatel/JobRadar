package dto

// CompanyVelocity holds per-company job counts for the current and prior week.
type CompanyVelocity struct {
	CompanyID   string `json:"company_id"`
	CompanyName string `json:"company_name"`
	ThisWeek    int    `json:"this_week"`
	LastWeek    int    `json:"last_week"`
	Delta       int    `json:"delta"`
}
