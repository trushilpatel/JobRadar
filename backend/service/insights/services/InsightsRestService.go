package services

import (
	"net/http"
	"strconv"

	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/insights/dto"
)

// RestService handles HTTP requests for the insights domain.
type RestService struct {
	next InsightsService
}

// NewRestService constructs a RestService.
func NewRestService(next InsightsService) *RestService {
	return &RestService{next: next}
}

func (h *RestService) TechStack(w http.ResponseWriter, r *http.Request) {
	data, err := h.next.TechStack(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, data)
}

func (h *RestService) CompanyActivity(w http.ResponseWriter, r *http.Request) {
	data, err := h.next.CompanyActivity(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, data)
}

func (h *RestService) Funnel(w http.ResponseWriter, r *http.Request) {
	data, err := h.next.Funnel(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, data)
}

func (h *RestService) Location(w http.ResponseWriter, r *http.Request) {
	data, err := h.next.Location(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, data)
}

func (h *RestService) Sponsorship(w http.ResponseWriter, r *http.Request) {
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		parsed, err := strconv.Atoi(d)
		if err != nil {
			handler.RespondValidationErrors(w, errors.ValidationErrors{{Field: "days", Code: "INVALID_VALUE", Message: "days must be a positive integer"}})
			return
		}
		days = parsed
	}

	data, err := h.next.Sponsorship(r.Context(), dto.InsightsFilter{Days: days})
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, data)
}

func (h *RestService) Velocity(w http.ResponseWriter, r *http.Request) {
	data, err := h.next.Velocity(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, data)
}

func (h *RestService) RoleCategories(w http.ResponseWriter, r *http.Request) {
	data, err := h.next.RoleCategories(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, data)
}
