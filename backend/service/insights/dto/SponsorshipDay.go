package dto

// SponsorshipDay holds daily sponsorship vs total enriched counts.
type SponsorshipDay struct {
	Date      string `json:"date"`
	Total     int    `json:"total"`
	Sponsored int    `json:"sponsored"`
}
