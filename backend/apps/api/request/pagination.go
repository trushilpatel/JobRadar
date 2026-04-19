package request

// Pagination holds validated page / limit values for list endpoints.
type Pagination struct {
	Page  int // min 1, default 1
	Limit int // min 1, max 100, default 20
}

// DefaultPagination returns the default pagination values.
func DefaultPagination() Pagination {
	return Pagination{Page: 1, Limit: 20}
}
