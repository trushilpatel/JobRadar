package dto

// StateCount holds job counts by US state.
type StateCount struct {
	State string `json:"state"`
	Count int    `json:"count"`
}
