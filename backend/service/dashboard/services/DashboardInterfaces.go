package services

// JobCountReader is the minimal interface needed to get job counts.
// GetJobCounts returns new, saved, applied counts respectively.
type JobCountReader interface {
	GetJobCounts() (newCount, savedCount, appliedCount int64, err error)
}
