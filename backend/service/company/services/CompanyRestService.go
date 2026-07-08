package services

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/apps/api/request"
	"github.com/trushilpatel/jobradar/apps/api/response"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
)

// RestService is the HTTP adapter for the public company chain. It depends
// only on CompanyService; validation and caching are handled by the
// decorator chain it wraps.
type RestService struct {
	next CompanyService
}

// NewRestService constructs the public RestService.
func NewRestService(next CompanyService) *RestService {
	return &RestService{next: next}
}

// listInput is the Huma input for GET /api/companies. Limit is read as a raw
// string (not a native int) and resolved via request.ParseLimit so an
// invalid or out-of-range value silently falls back to the default rather
// than failing validation — matching the pre-Huma behavior exactly.
type listInput struct {
	Search  string `query:"search" doc:"ILIKE match on company name"`
	SortBy  string `query:"sort_by" doc:"Sort column (defaults to name if unrecognized)"`
	SortDir string `query:"sort_dir" doc:"asc or desc"`
	Cursor  string `query:"cursor" doc:"Opaque keyset pagination cursor"`
	Limit   string `query:"limit" doc:"Page size 1-100; invalid or missing values default to 20" example:"20"`

	limit int
}

// Resolve computes the clamped limit after Huma parses the raw query
// string, mirroring apps/api/request.ParsePagination's exact fallback
// semantics.
func (in *listInput) Resolve(_ huma.Context) []error {
	in.limit = request.ParseLimit(in.Limit, request.DefaultPagination().Limit)
	return nil
}

// getInput is the Huma input for GET /api/companies/{id}.
type getInput struct {
	ID string `path:"id"`
}

// list handles GET /api/companies.
func (h *RestService) list(ctx context.Context, in *listInput) (*apihuma.CursorPageOutput[companydto.CompanyPublicView], error) {
	filter := companydto.CompanyFilter{
		Search:  in.Search,
		SortBy:  in.SortBy,
		SortDir: in.SortDir,
		Cursor:  in.Cursor,
	}

	result, err := h.next.ListPublic(ctx, filter, in.limit)
	if err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.CursorPageOutput[companydto.CompanyPublicView]{}
	out.Body = apihuma.CursorPage[companydto.CompanyPublicView]{
		Data:       result.Items,
		Total:      result.Total,
		Limit:      in.limit,
		NextCursor: result.NextCursor,
	}
	return out, nil
}

// get handles GET /api/companies/{id}.
func (h *RestService) get(ctx context.Context, in *getInput) (*apihuma.SingleOutput[companydto.CompanyPublicView], error) {
	view, err := h.next.GetPublic(ctx, in.ID)
	if err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.SingleOutput[companydto.CompanyPublicView]{}
	out.Body = response.Response[companydto.CompanyPublicView]{Data: *view}
	return out, nil
}
