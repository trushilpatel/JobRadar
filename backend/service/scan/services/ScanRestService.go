package services

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// RestService handles HTTP requests for the scan domain.
// It holds a single next Service dependency; routing belongs in RestRouter.
type RestService struct {
	next Service
}

// NewRestService constructs a RestService.
func NewRestService(next Service) *RestService {
	return &RestService{next: next}
}

// GET /companies/queue
func (h *RestService) ListQueue(w http.ResponseWriter, r *http.Request) {
	filter := dto.QueueFilter{
		Status: r.URL.Query().Get("status"),
		Source: r.URL.Query().Get("source"),
	}

	views, err := h.next.ListQueue(r.Context(), filter)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, views)
}

// DELETE /companies/queue/:entry_id
func (h *RestService) RemoveFromQueue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "entry_id")
	if err := h.next.RemoveFromQueue(r.Context(), id); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]string{"id": id})
}

// POST /companies/queue/:entry_id/move-up
func (h *RestService) MoveQueueItemUp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "entry_id")
	if err := h.next.MoveUp(r.Context(), id); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]string{"id": id})
}

// POST /companies/queue/:entry_id/move-down
func (h *RestService) MoveQueueItemDown(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "entry_id")
	if err := h.next.MoveDown(r.Context(), id); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]string{"id": id})
}

// POST /companies/scan-all
func (h *RestService) TriggerScanAll(w http.ResponseWriter, r *http.Request) {
	count, err := h.next.TriggerScanAll(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusAccepted, map[string]any{"queued": count})
}

// GET /companies/:id/scan-stats
func (h *RestService) ScanStats(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	stats, err := h.next.GetStats(r.Context(), id)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, stats)
}

// POST /companies/:id/scan
func (h *RestService) TriggerScan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.next.TriggerScan(r.Context(), id); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusAccepted, map[string]string{"company_id": id, "status": "scan_queued"})
}
