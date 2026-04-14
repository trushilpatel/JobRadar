package dashboard

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/api/handler"
)

// Handler handles HTTP requests for the dashboard domain.
type Handler struct {
	validator Validator
	dashboard Service
}

// NewHandler constructs a Handler.
func NewHandler(v Validator, svc Service) *Handler {
	return &Handler{validator: v, dashboard: svc}
}

// Register mounts all dashboard routes onto r.
func (h *Handler) Register(r chi.Router) {
	r.Get("/dashboard/stats", h.Stats)
	r.Get("/dashboard/activity", h.Activity)
}

// GET /dashboard/stats
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.dashboard.GetStats()
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, stats)
}

// GET /dashboard/activity?days=7
func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	days := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil {
			days = parsed
		}
	}

	if err := h.validator.ValidateActivity(days); err != nil {
		handler.HandleError(w, err)
		return
	}

	activity, err := h.dashboard.GetActivity(days)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, activity)
}
