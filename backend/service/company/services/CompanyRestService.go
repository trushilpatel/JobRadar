package services

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/apps/api/request"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
)

// publicListEnvelope is the JSON envelope for GET /api/companies. Carries
// the cursor-paged result set with a next_cursor for the subsequent page.
type publicListEnvelope struct {
	Data       []companydto.CompanyPublicView `json:"data"`
	Total      int64                          `json:"total"`
	Limit      int                            `json:"limit"`
	NextCursor *string                        `json:"next_cursor"`
}

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

// list handles GET /api/companies?search=...&sort_by=...&sort_dir=...&cursor=...&limit=...
func (h *RestService) list(w http.ResponseWriter, r *http.Request) {
	p := request.ParsePagination(r)
	q := r.URL.Query()

	var filter companydto.CompanyFilter
	if v := q.Get("search"); v != "" {
		filter.Search = v
	}
	if v := q.Get("sort_by"); v != "" {
		filter.SortBy = v
	}
	if v := q.Get("sort_dir"); v != "" {
		filter.SortDir = v
	}
	if v := q.Get("cursor"); v != "" {
		filter.Cursor = v
	}

	result, err := h.next.ListPublic(r.Context(), filter, p.Limit)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, publicListEnvelope{
		Data:       result.Items,
		Total:      result.Total,
		Limit:      p.Limit,
		NextCursor: result.NextCursor,
	})
}

// get handles GET /api/companies/{id}.
func (h *RestService) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	view, err := h.next.GetPublic(r.Context(), id)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, view)
}
