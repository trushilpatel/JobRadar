package dto

// CompanyActivityItem holds weekly job count per company.
type CompanyActivityItem struct {
	CompanyName string `json:"company_name"`
	Week        string `json:"week"`
	Count       int    `json:"count"`
}
