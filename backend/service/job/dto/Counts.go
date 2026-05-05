package dto

// Counts holds the per-user count of jobs in each user_job_state status
// bucket. "New" is computed elsewhere (it is the absence of a row), so it
// is not a stored count — callers wanting "new" derive it from total jobs
// minus the sum of these stored statuses.
type Counts struct {
	Saved        int64 `json:"saved"`
	Applied      int64 `json:"applied"`
	Dismissed    int64 `json:"dismissed"`
	Interviewing int64 `json:"interviewing"`
	Offer        int64 `json:"offer"`
	Rejected     int64 `json:"rejected"`
}

// CountsRow is the raw GROUP BY status row returned by the per-user count
// query — internal to the repository.
type CountsRow struct {
	Status string
	Count  int64
}
