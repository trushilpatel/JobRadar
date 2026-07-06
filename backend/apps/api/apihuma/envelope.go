// Package apihuma is the foundation package for the chi->Huma migration
// (docs/superpowers/plans/2026-06-13-huma-migration-plan.md, Phase 0). It
// provides Huma operation output types that reuse or exactly mirror the
// pre-Huma JSON envelopes, and error mapping that preserves the legacy error
// shape — so migrated endpoints are byte-identical to their pre-migration
// responses and the frontend needs zero changes during the migration.
package apihuma

import "github.com/trushilpatel/jobradar/apps/api/response"

// SingleOutput is the Huma output body for a single-resource response. It
// reuses response.Response[T] directly (the exact type handler.Respond
// emits today), so the JSON body is byte-identical: {"data": T}.
type SingleOutput[T any] struct {
	Body response.Response[T]
}

// PagedOutput is the Huma output body for classic page+offset lists. It
// reuses response.PagedResponse[T] directly (the exact type
// handler.RespondPaged emits today): {"data": [T], "total", "page", "limit"}.
type PagedOutput[T any] struct {
	Body response.PagedResponse[T]
}

// CursorPage is the cursor-paginated list envelope used by public list
// endpoints (e.g. GET /api/companies today via CompanyRestService's
// publicListEnvelope): {"data": [T], "total", "limit", "next_cursor"}. Field
// order matches the legacy struct exactly.
type CursorPage[T any] struct {
	Data       []T     `json:"data"`
	Total      int64   `json:"total"`
	Limit      int     `json:"limit"`
	NextCursor *string `json:"next_cursor"`
}

// CursorPageOutput wraps CursorPage as a Huma operation output body.
type CursorPageOutput[T any] struct {
	Body CursorPage[T]
}

// AdminCursorPage is the cursor-paginated list envelope used by admin list
// endpoints that also report an active-count header stat (e.g. GET
// /admin/companies today via CompanyAdminRestService's adminListEnvelope).
// Field order matches the legacy struct exactly (total_active between total
// and limit) so migrated output is byte-identical.
type AdminCursorPage[T any] struct {
	Data        []T     `json:"data"`
	Total       int64   `json:"total"`
	TotalActive int64   `json:"total_active"`
	Limit       int     `json:"limit"`
	NextCursor  *string `json:"next_cursor"`
}

// AdminCursorPageOutput wraps AdminCursorPage as a Huma operation output body.
type AdminCursorPageOutput[T any] struct {
	Body AdminCursorPage[T]
}
