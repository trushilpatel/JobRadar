package dto

// PageResult is the keyset-pagination response shape for the job feed list.
// NextCursor is empty when no more pages are available; Total is omitted
// (keyset pagination doesn't compute totals — counts come from a separate
// /jobs/counts call).
type PageResult struct {
	Items      []JobView `json:"items"`
	NextCursor string    `json:"next_cursor"`
}
