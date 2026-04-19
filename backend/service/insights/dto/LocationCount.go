package dto

// LocationCount holds job counts by country.
type LocationCount struct {
	Country string `json:"country"`
	Count   int    `json:"count"`
}
