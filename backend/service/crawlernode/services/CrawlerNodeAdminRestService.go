package services

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
)

// AdminRestService is the HTTP adapter for admin-facing crawler-node endpoints.
// Depends only on NodeService; validation is handled by the decorator chain.
type AdminRestService struct {
	next NodeService
}

// NewAdminRestService constructs the admin-facing HTTP adapter.
func NewAdminRestService(next NodeService) *AdminRestService {
	return &AdminRestService{next: next}
}

// provisionNodeRequest is the JSON body for POST /admin/crawler-nodes.
type provisionNodeRequest struct {
	Name string `json:"name"`
}

// setNodeStatusRequest is the JSON body for PATCH /admin/crawler-nodes/{id}.
type setNodeStatusRequest struct {
	Status string `json:"status"`
}

// ListNodes handles GET /admin/crawler-nodes?status=<optional>
func (h *AdminRestService) ListNodes(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	nodes, err := h.next.ListNodes(r.Context(), status)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, nodes)
}

// ProvisionNode handles POST /admin/crawler-nodes
func (h *AdminRestService) ProvisionNode(w http.ResponseWriter, r *http.Request) {
	req, ok := handler.DecodeBody[provisionNodeRequest](w, r)
	if !ok {
		return
	}
	resp, err := h.next.ProvisionNode(r.Context(), req.Name)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusCreated, resp)
}

// SetNodeStatus handles PATCH /admin/crawler-nodes/{id}
func (h *AdminRestService) SetNodeStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := handler.DecodeBody[setNodeStatusRequest](w, r)
	if !ok {
		return
	}
	if err := h.next.SetNodeStatus(r.Context(), id, req.Status); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, struct{}{})
}

// RetireNode handles DELETE /admin/crawler-nodes/{id}
func (h *AdminRestService) RetireNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.next.RetireNode(r.Context(), id); err != nil {
		handler.HandleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
