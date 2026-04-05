package handler

import (
	"net/http"

	"github.com/trushilpatel/jobradar/internal/store"
)

type InsightsHandler struct {
	insights store.InsightsReader
}

func NewInsightsHandler(insights store.InsightsReader) *InsightsHandler {
	return &InsightsHandler{insights: insights}
}

// GET /insights/tech-stack
func (h *InsightsHandler) TechStack(w http.ResponseWriter, r *http.Request) {
	data, err := h.insights.GetTechStackTrends(15)
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
