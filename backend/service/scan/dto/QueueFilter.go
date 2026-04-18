package dto

// QueueFilter holds optional filter criteria for listing the scan queue.
type QueueFilter struct {
	Status string // "queued" | "processing"
	Source string // "manual" | "scheduler"
}
