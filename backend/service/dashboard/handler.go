package dashboard

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/api/handler"
)

// Handler handles HTTP requests for the dashboard domain.
type Handler struct {
	validator       Validator
	dashboard       Service
	userDashboard   UserDashboardService
	pipelineService PipelineService
}

// NewHandler constructs a Handler.
func NewHandler(v Validator, svc Service, userSvc UserDashboardService, pipelineSvc PipelineService) *Handler {
	return &Handler{validator: v, dashboard: svc, userDashboard: userSvc, pipelineService: pipelineSvc}
}

// Register mounts all dashboard routes onto r.
func (h *Handler) Register(r chi.Router) {
	// Existing routes — unchanged
	r.Get("/dashboard/stats", h.Stats)
	r.Get("/dashboard/activity", h.Activity)

	// User dashboard routes
	r.Get("/dashboard/progress", h.Progress)
	r.Get("/dashboard/market", h.Market)
	r.Get("/dashboard/heatmap", h.Heatmap)
	r.Get("/dashboard/interviews", h.Interviews)
	r.Get("/dashboard/velocity", h.Velocity)
	r.Get("/dashboard/tech-trends", h.TechTrends)

	// Pipeline strip
	r.Get("/dashboard/pipeline", h.Pipeline)
}

// GET /dashboard/stats
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.dashboard.GetStats(r.Context())
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

	activity, err := h.dashboard.GetActivity(r.Context(), days)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, activity)
}

// GET /dashboard/progress
func (h *Handler) Progress(w http.ResponseWriter, r *http.Request) {
	stats, err := h.userDashboard.GetProgressStats(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, stats)
}

// GET /dashboard/market
func (h *Handler) Market(w http.ResponseWriter, r *http.Request) {
	stats, err := h.userDashboard.GetMarketStats(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, stats)
}

// GET /dashboard/heatmap
func (h *Handler) Heatmap(w http.ResponseWriter, r *http.Request) {
	counts, err := h.userDashboard.GetHeatmap(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, counts)
}

// GET /dashboard/interviews
func (h *Handler) Interviews(w http.ResponseWriter, r *http.Request) {
	rows, err := h.userDashboard.GetInterviews(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, rows)
}

// GET /dashboard/velocity?limit=10
func (h *Handler) Velocity(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	rows, err := h.userDashboard.GetVelocity(r.Context(), limit)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, rows)
}

// GET /dashboard/tech-trends
func (h *Handler) TechTrends(w http.ResponseWriter, r *http.Request) {
	trends, err := h.userDashboard.GetTechTrends(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, trends)
}

// GET /dashboard/pipeline
func (h *Handler) Pipeline(w http.ResponseWriter, r *http.Request) {
	stats, err := h.pipelineService.GetPipelineStats(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, stats)
}
