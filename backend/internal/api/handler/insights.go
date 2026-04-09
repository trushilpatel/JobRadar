package handler

import (
	"net/http"
	"strconv"

	"github.com/trushilpatel/jobradar/internal/store"
)

type InsightsHandler struct {
	insights store.InsightsReader
}

func NewInsightsHandler(insights store.InsightsReader) *InsightsHandler {
	return &InsightsHandler{insights: insights}
}

// GET /insights/tech-stack?limit=15
func (h *InsightsHandler) TechStack(w http.ResponseWriter, r *http.Request) {
	limit := 15
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	data, err := h.insights.GetTechStackTrends(limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch tech stack")
		return
	}
	respond(w, http.StatusOK, data)
}

// GET /insights/company-activity
func (h *InsightsHandler) CompanyActivity(w http.ResponseWriter, r *http.Request) {
	data, err := h.insights.GetCompanyActivity()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch company activity")
		return
	}
	respond(w, http.StatusOK, data)
}

// GET /insights/funnel
func (h *InsightsHandler) Funnel(w http.ResponseWriter, r *http.Request) {
	data, err := h.insights.GetPipelineFunnel()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch funnel")
		return
	}
	respond(w, http.StatusOK, data)
}

// GET /insights/location
func (h *InsightsHandler) Location(w http.ResponseWriter, r *http.Request) {
	countries, err := h.insights.GetLocationBreakdown()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch location breakdown")
		return
	}
	states, err := h.insights.GetUSAStateBreakdown()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch state breakdown")
		return
	}
	respond(w, http.StatusOK, map[string]any{
		"countries": countries,
		"states":    states,
	})
}

// GET /insights/sponsorship?days=30
func (h *InsightsHandler) Sponsorship(w http.ResponseWriter, r *http.Request) {
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}
	data, err := h.insights.GetSponsorshipTrend(days)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch sponsorship trend")
		return
	}
	respond(w, http.StatusOK, data)
}

// GET /insights/velocity?limit=20
func (h *InsightsHandler) Velocity(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	data, err := h.insights.GetCompanyVelocity(limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch company velocity")
		return
	}
	respond(w, http.StatusOK, data)
}

// GET /insights/role-categories
func (h *InsightsHandler) RoleCategories(w http.ResponseWriter, r *http.Request) {
	data, err := h.insights.GetRoleCategoryBreakdown()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch role categories")
		return
	}
	respond(w, http.StatusOK, data)
}
