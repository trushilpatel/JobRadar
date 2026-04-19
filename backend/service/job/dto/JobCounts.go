package dto

// JobCounts holds the count of jobs per active status.
type JobCounts struct {
	New          int64 `json:"new"`
	Saved        int64 `json:"saved"`
	Applied      int64 `json:"applied"`
	Interviewing int64 `json:"interviewing"`
	Offer        int64 `json:"offer"`
}
