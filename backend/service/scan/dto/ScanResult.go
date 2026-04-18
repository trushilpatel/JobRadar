package dto

import "time"

// ScanResult carries the outcome of one company crawl.
type ScanResult struct {
	FetchedCount int
	NewCount     int
	Duration     time.Duration
	ErrMsg       string // empty string means success
}
