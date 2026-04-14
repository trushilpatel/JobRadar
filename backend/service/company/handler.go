package company

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/api/handler"
	"github.com/trushilpatel/jobradar/api/request"
)

// Handler handles HTTP requests for the company domain.
type Handler struct {
	validator Validator
	companies Service
}

// NewHandler constructs a Handler.
func NewHandler(v Validator, svc Service) *Handler {
	return &Handler{validator: v, companies: svc}
}

// Register mounts all company routes (except scan/queue) onto r.
func (h *Handler) Register(r chi.Router) {
	r.Route("/companies", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Patch("/active", h.SetAllActive)
		r.Patch("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)
	})
}

// companyListResponse is the envelope for GET /companies.
// Extends the standard paged envelope with total_active for header stats.
type companyListResponse struct {
	Data        []CompanyView `json:"data"`
	Total       int64         `json:"total"`
	TotalActive int64         `json:"total_active"`
	Page        int           `json:"page"`
	Limit       int           `json:"limit"`
}

// GET /companies?active=true&adapter_type=greenhouse&search=stripe&page=1&limit=20
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	p := request.ParsePagination(r)
	q := r.URL.Query()

	var filter CompanyFilter
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

	if err := h.validator.ValidateList(filter, p); err != nil {
		handler.HandleError(w, err)
		return
	}

	result, err := h.companies.List(filter, p)
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
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var input CreateCompanyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.validator.ValidateCreate(input); err != nil {
		handler.HandleError(w, err)
		return
	}

	company, err := h.companies.Create(input)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusCreated, company)
}

// PATCH /companies/active
func (h *Handler) SetAllActive(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Active *bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.validator.ValidateSetAllActive(body.Active); err != nil {
		handler.HandleError(w, err)
		return
	}

	if err := h.companies.SetAllActive(*body.Active); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]bool{"active": *body.Active})
}

// PATCH /companies/:id
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateCompanyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.validator.ValidateUpdate(input); err != nil {
		handler.HandleError(w, err)
		return
	}

	if err := h.companies.Update(id, input); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"id": id})
}

// DELETE /companies/:id
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.companies.Delete(id); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"id": id})
}
