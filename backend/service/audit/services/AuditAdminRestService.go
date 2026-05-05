package services

import (
	"net/http"

	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/apps/api/request"
	auditmodel "github.com/trushilpatel/jobradar/service/audit/models"
)

// AdminRestService is the HTTP adapter for the admin audit log. It owns
// request decoding and response shaping for GET /admin/audit-log; routing
// is the responsibility of AuditAdminRestRouter.
type AdminRestService struct {
	next Service
}

// NewAdminRestService constructs the admin audit HTTP adapter.
func NewAdminRestService(next Service) *AdminRestService {
	if next == nil {
		panic("audit.NewAdminRestService: next must not be nil")
	}
	return &AdminRestService{next: next}
}

// list handles GET /admin/audit-log. Accepts optional query params:
// actor_user_id, action, page, limit.
func (h *AdminRestService) list(w http.ResponseWriter, r *http.Request) {
	p := request.ParsePagination(r)
	q := r.URL.Query()

	filter := ListFilter{
		Limit:  p.Limit,
		Offset: (p.Page - 1) * p.Limit,
	}
	if v := q.Get("actor_user_id"); v != "" {
		filter.ActorUserID = &v
	}
	if v := q.Get("action"); v != "" {
		filter.Action = &v
	}

	entries, total, err := h.next.List(r.Context(), filter)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	if entries == nil {
		entries = []auditmodel.Entry{}
	}
	handler.RespondPaged(w, http.StatusOK, entries, int(total), p.Page, p.Limit)
}
