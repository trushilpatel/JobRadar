package dto

// BulkDismissInput is the request payload for dismissing multiple jobs in one call.
type BulkDismissInput struct {
	IDs    []string `json:"ids"`
	Reason string   `json:"reason"`
}
