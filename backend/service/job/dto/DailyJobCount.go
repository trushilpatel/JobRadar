package dto

// DailyJobCount holds the number of jobs added on a given date.
type DailyJobCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}
