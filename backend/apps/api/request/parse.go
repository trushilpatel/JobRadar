package request

import (
	"net/http"
	"strconv"
)

// ParsePage parses a raw "page" query value, falling back to defaultPage
// when raw is empty, not a valid integer, or less than 1. Exported (rather
// than inlined in ParsePagination) so Huma input Resolvers can apply the
// exact same fallback semantics when reading the raw string themselves —
// see apps/api/apihuma.
func ParsePage(raw string, defaultPage int) int {
	if raw == "" {
		return defaultPage
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return defaultPage
	}
	return n
}

// ParseLimit parses a raw "limit" query value, clamping to [1,100] and
// falling back to defaultLimit when raw is empty or not a valid integer.
// Exported for the same reason as ParsePage.
func ParseLimit(raw string, defaultLimit int) int {
	if raw == "" {
		return defaultLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return defaultLimit
	}
	if n < 1 {
		n = 1
	}
	if n > 100 {
		n = 100
	}
	return n
}

// ParsePagination extracts ?page=N&limit=N from the request query string.
// Missing or invalid values fall back to defaults. Limit is clamped to 1–100.
func ParsePagination(r *http.Request) Pagination {
	p := DefaultPagination()
	p.Page = ParsePage(r.URL.Query().Get("page"), p.Page)
	p.Limit = ParseLimit(r.URL.Query().Get("limit"), p.Limit)
	return p
}
