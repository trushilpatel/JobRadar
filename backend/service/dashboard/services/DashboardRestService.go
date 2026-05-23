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
}

// NewDashboardRestService constructs a RestService.
func NewDashboardRestService(dashboard DashboardService, userDashboard UserDashboardService) *RestService {
	return &RestService{
		dashboard:     dashboard,
		userDashboard: userDashboard,
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
		parsed, err := strconv.Atoi(d)
		if err != nil {
			handler.HandleError(w, &errors.ValidationError{
				Field: "days", Code: "INVALID_FORMAT", Message: "days must be an integer",
			})
			return
		}
		days = parsed
	}
	if days < 1 || days > 365 {
		handler.HandleError(w, &errors.ValidationError{
			Field: "days", Code: "INVALID_VALUE", Message: "days must be between 1 and 365",
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
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed < 1 || parsed > 100 {
			handler.HandleError(w, &errors.ValidationError{
				Field: "limit", Code: "INVALID_VALUE", Message: "limit must be between 1 and 100",
			})
			return
		}
		limit = parsed
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
