package dto

// QueueFilter holds optional filter criteria for listing the scan queue.
type QueueFilter struct {
	Status string // "queued" | "running"
	Source string // "manual" | "scheduler"
}
