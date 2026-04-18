package scan

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/api/handler"
)

// Handler handles HTTP requests for the scan domain.
// It mounts under /companies alongside the company handler.
type Handler struct {
	validator Validator
	scans     Service
}

// NewHandler constructs a Handler.
func NewHandler(v Validator, svc Service) *Handler {
	return &Handler{validator: v, scans: svc}
}

// Register mounts scan-specific routes under /companies.
// Routes are mounted at the top level (not as a sub-router) to avoid conflicts
// when both CompanyHandler and ScanHandler mount under /companies.
func (h *Handler) Register(r chi.Router) {
	// Queue management
	r.Get("/companies/queue", h.ListQueue)
	r.Delete("/companies/queue/{entry_id}", h.RemoveFromQueue)
	r.Post("/companies/queue/{entry_id}/move-up", h.MoveQueueItemUp)
	r.Post("/companies/queue/{entry_id}/move-down", h.MoveQueueItemDown)

	// Bulk scan
	r.Post("/companies/scan-all", h.TriggerScanAll)

	// Per-company scan operations
	r.Get("/companies/{id}/scan-stats", h.ScanStats)
	r.Post("/companies/{id}/scan", h.TriggerScan)
}

// GET /companies/queue
func (h *Handler) ListQueue(w http.ResponseWriter, r *http.Request) {
	filter := QueueFilter{
		Status: r.URL.Query().Get("status"),
		Source: r.URL.Query().Get("source"),
	}

	if err := h.validator.ValidateListQueue(filter); err != nil {
		handler.HandleError(w, err)
		return
	}

	views, err := h.scans.ListQueue(r.Context(), filter)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, views)
}

// DELETE /companies/queue/:entry_id
func (h *Handler) RemoveFromQueue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "entry_id")
	if err := h.scans.RemoveFromQueue(r.Context(), id); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]string{"id": id})
}

// POST /companies/queue/:entry_id/move-up
func (h *Handler) MoveQueueItemUp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "entry_id")
	if err := h.scans.MoveUp(r.Context(), id); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]string{"id": id})
}

// POST /companies/queue/:entry_id/move-down
func (h *Handler) MoveQueueItemDown(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "entry_id")
	if err := h.scans.MoveDown(r.Context(), id); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]string{"id": id})
}

// POST /companies/scan-all
func (h *Handler) TriggerScanAll(w http.ResponseWriter, r *http.Request) {
	count, err := h.scans.TriggerScanAll(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusAccepted, map[string]any{"queued": count})
}

// GET /companies/:id/scan-stats
func (h *Handler) ScanStats(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.validator.ValidateTriggerScan(id); err != nil {
		handler.HandleError(w, err)
		return
	}

	stats, err := h.scans.GetStats(r.Context(), id)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, stats)
}

// POST /companies/:id/scan
func (h *Handler) TriggerScan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.validator.ValidateTriggerScan(id); err != nil {
		handler.HandleError(w, err)
		return
	}

	if err := h.scans.TriggerScan(r.Context(), id); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusAccepted, map[string]string{"company_id": id, "status": "scan_queued"})
}
