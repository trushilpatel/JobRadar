package insights

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/api/handler"
)

// Handler handles HTTP requests for the insights domain.
type Handler struct {
	validator Validator
	insights  Service
}

// NewHandler constructs a Handler.
func NewHandler(v Validator, svc Service) *Handler {
	return &Handler{validator: v, insights: svc}
}

// Register mounts all insights routes onto r.
func (h *Handler) Register(r chi.Router) {
	r.Get("/insights/tech-stack", h.TechStack)
	r.Get("/insights/company-activity", h.CompanyActivity)
	r.Get("/insights/funnel", h.Funnel)
	r.Get("/insights/location", h.Location)
	r.Get("/insights/sponsorship", h.Sponsorship)
	r.Get("/insights/velocity", h.Velocity)
	r.Get("/insights/role-categories", h.RoleCategories)
}

func (h *Handler) TechStack(w http.ResponseWriter, r *http.Request) {
	data, err := h.insights.TechStack(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, data)
}

func (h *Handler) CompanyActivity(w http.ResponseWriter, r *http.Request) {
	data, err := h.insights.CompanyActivity(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, data)
}

func (h *Handler) Funnel(w http.ResponseWriter, r *http.Request) {
	data, err := h.insights.Funnel(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, data)
}

func (h *Handler) Location(w http.ResponseWriter, r *http.Request) {
	data, err := h.insights.Location(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, data)
}

func (h *Handler) Sponsorship(w http.ResponseWriter, r *http.Request) {
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil {
			days = parsed
		}
	}

	filter := InsightsFilter{Days: days}
	if err := h.validator.ValidateFilter(filter); err != nil {
		handler.HandleError(w, err)
		return
	}

	data, err := h.insights.Sponsorship(r.Context(), filter)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, data)
}

func (h *Handler) Velocity(w http.ResponseWriter, r *http.Request) {
	data, err := h.insights.Velocity(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, data)
}

func (h *Handler) RoleCategories(w http.ResponseWriter, r *http.Request) {
	data, err := h.insights.RoleCategories(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, data)
}
