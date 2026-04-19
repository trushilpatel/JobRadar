package dto

// TechCount holds the count of jobs mentioning a given technology.
type TechCount struct {
	Tech  string `json:"tech"`
	Count int    `json:"count"`
}
