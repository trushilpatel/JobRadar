package services

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/api/handler"
	"github.com/trushilpatel/jobradar/api/request"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
)

// RestService handles HTTP requests for the company domain.
// It has a single dependency on Service; validation is handled by the chain.
type RestService struct {
	next Service
}

// NewRestService constructs a RestService.
func NewRestService(next Service) *RestService {
	return &RestService{next: next}
}

// companyListResponse is the envelope for GET /companies.
// Extends the standard paged envelope with total_active for header stats.
type companyListResponse struct {
	Data        []companydto.CompanyView `json:"data"`
	Total       int64                    `json:"total"`
	TotalActive int64                    `json:"total_active"`
	Page        int                      `json:"page"`
	Limit       int                      `json:"limit"`
}

// GET /companies?active=true&adapter_type=greenhouse&search=stripe&page=1&limit=20
func (h *RestService) List(w http.ResponseWriter, r *http.Request) {
	p := request.ParsePagination(r)
	q := r.URL.Query()

	var filter companydto.CompanyFilter
	if v := q.Get("adapter_type"); v != "" {
		filter.AdapterType = v
	}
	if v := q.Get("active"); v != "" {
		active := v == "true"
		filter.Active = &active
	}
	if v := q.Get("search"); v != "" {
		filter.Search = v
	}

	result, err := h.next.List(r.Context(), filter, p.Page, p.Limit)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, companyListResponse{
		Data:        result.Items,
		Total:       result.Total,
		TotalActive: result.TotalActive,
		Page:        p.Page,
		Limit:       p.Limit,
	})
}

// POST /companies
func (h *RestService) Create(w http.ResponseWriter, r *http.Request) {
	var input companydto.CreateCompanyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	company, err := h.next.Create(r.Context(), input)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusCreated, company)
}

// PATCH /companies/active
func (h *RestService) SetAllActive(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Active *bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if body.Active == nil {
		handler.RespondError(w, http.StatusBadRequest, "REQUIRED", "active (bool) is required")
		return
	}

	if err := h.next.SetAllActive(r.Context(), *body.Active); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]bool{"active": *body.Active})
}

// PATCH /companies/:id
func (h *RestService) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input companydto.UpdateCompanyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.next.Update(r.Context(), id, input); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"id": id})
}

// DELETE /companies/:id
func (h *RestService) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.next.Delete(r.Context(), id); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"id": id})
}
