package dto

// MarketStats holds aggregated market stats for the Market panel.
type MarketStats struct {
	NewSinceYesterday int64 `json:"new_since_yesterday"`
	TotalOpen         int64 `json:"total_open"`
	AddedThisWeek     int64 `json:"added_this_week"`
	AddedLastWeek     int64 `json:"added_last_week"`
	WeeklyDelta       int64 `json:"weekly_delta"`
}
