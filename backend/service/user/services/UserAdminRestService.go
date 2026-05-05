package services

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/apps/api/request"
	"github.com/trushilpatel/jobradar/common/identity"
)

// AdminRestService is the HTTP adapter for the admin user chain. It owns
// request decoding and response shaping for the /admin/users endpoints;
// routing is the responsibility of UserAdminRestRouter.
type AdminRestService struct {
	next AdminService
}

// NewAdminRestService constructs the admin HTTP adapter.
func NewAdminRestService(next AdminService) *AdminRestService {
	if next == nil {
		panic("user.NewAdminRestService: next must not be nil")
	}
	return &AdminRestService{next: next}
}

// list handles GET /admin/users. Accepts optional ?status=&email=&page=&limit=
func (h *AdminRestService) list(w http.ResponseWriter, r *http.Request) {
	p := request.ParsePagination(r)
	q := r.URL.Query()

	var filter AdminUserFilter
	if v := q.Get("status"); v != "" {
		filter.Status = &v
	}
	filter.Email = q.Get("email")

	result, err := h.next.List(r.Context(), filter, p.Page, p.Limit)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	items := result.Items
	if items == nil {
		items = []UserWithRoles{}
	}
	handler.RespondPaged(w, http.StatusOK, items, int(result.Total), p.Page, p.Limit)
}

// get handles GET /admin/users/{id}.
func (h *AdminRestService) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	view, err := h.next.GetWithRoles(r.Context(), id)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, view)
}

// setRolesRequest is the body for PATCH /admin/users/{id}/roles.
type setRolesRequest struct {
	RoleIDs []string `json:"role_ids"`
}

// setRoles handles PATCH /admin/users/{id}/roles.
func (h *AdminRestService) setRoles(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	grantedBy := identity.UserIDFrom(r.Context())

	var body setRolesRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if body.RoleIDs == nil {
		body.RoleIDs = []string{}
	}

	if err := h.next.SetRoles(r.Context(), id, body.RoleIDs, grantedBy); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]string{"id": id})
}

// setStatusRequest is the body for PATCH /admin/users/{id}/status.
type setStatusRequest struct {
	Status string `json:"status"`
}

// setStatus handles PATCH /admin/users/{id}/status.
func (h *AdminRestService) setStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body setStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if body.Status == "" {
		handler.RespondError(w, http.StatusBadRequest, "REQUIRED", "status is required")
		return
	}

	if err := h.next.SetStatus(r.Context(), id, body.Status); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]string{"id": id, "status": body.Status})
}

// delete handles DELETE /admin/users/{id}.
func (h *AdminRestService) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.next.AdminDelete(r.Context(), id); err != nil {
		handler.HandleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
