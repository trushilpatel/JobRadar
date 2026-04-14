package request

import (
	"net/http"
	"strconv"
)

// ParsePagination extracts ?page=N&limit=N from the request query string.
// Missing or invalid values fall back to defaults. Limit is clamped to 1–100.
func ParsePagination(r *http.Request) Pagination {
	p := DefaultPagination()

	if raw := r.URL.Query().Get("page"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 1 {
			p.Page = n
		}
	}

	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			if n < 1 {
				n = 1
			}
			if n > 100 {
				n = 100
			}
			p.Limit = n
		}
	}

	return p
}
