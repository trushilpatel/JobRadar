package services

import (
	"net/http"
	"strconv"

	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/common/errors"
)

// RestService handles HTTP requests for the dashboard domain.
type RestService struct {
	dashboard     DashboardService
	userDashboard UserDashboardService
	pipeline      PipelineService
}

// NewDashboardRestService constructs a RestService.
func NewDashboardRestService(dashboard DashboardService, userDashboard UserDashboardService, pipeline PipelineService) *RestService {
	return &RestService{
		dashboard:     dashboard,
		userDashboard: userDashboard,
		pipeline:      pipeline,
	}
}

// GET /dashboard/stats
func (h *RestService) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.dashboard.GetStats(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, stats)
}

// GET /dashboard/activity?days=7
func (h *RestService) Activity(w http.ResponseWriter, r *http.Request) {
	days := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil {
			days = parsed
		}
	}

	if days < 1 {
		handler.HandleError(w, &errors.ValidationError{
			Field:   "days",
			Code:    "INVALID_VALUE",
			Message: "days must be >= 1",
		})
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
func (h *RestService) Progress(w http.ResponseWriter, r *http.Request) {
	stats, err := h.userDashboard.GetProgressStats(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, stats)
}

// GET /dashboard/market
func (h *RestService) Market(w http.ResponseWriter, r *http.Request) {
	stats, err := h.userDashboard.GetMarketStats(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, stats)
}

// GET /dashboard/heatmap
func (h *RestService) Heatmap(w http.ResponseWriter, r *http.Request) {
	counts, err := h.userDashboard.GetHeatmap(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, counts)
}

// GET /dashboard/interviews
func (h *RestService) Interviews(w http.ResponseWriter, r *http.Request) {
	rows, err := h.userDashboard.GetInterviews(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, rows)
}

// GET /dashboard/velocity?limit=10
func (h *RestService) Velocity(w http.ResponseWriter, r *http.Request) {
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
func (h *RestService) TechTrends(w http.ResponseWriter, r *http.Request) {
	trends, err := h.userDashboard.GetTechTrends(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, trends)
}

// GET /dashboard/pipeline
func (h *RestService) Pipeline(w http.ResponseWriter, r *http.Request) {
	stats, err := h.pipeline.GetPipelineStats(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, stats)
}
