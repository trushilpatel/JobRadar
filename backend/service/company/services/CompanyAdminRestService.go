package services

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/apps/api/request"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
)

// AdminRestService is the HTTP adapter for the admin company chain. It
// depends only on CompanyAdminService; validation and cache invalidation
// are handled by the decorator chain.
type AdminRestService struct {
	next CompanyAdminService
}

// NewAdminRestService constructs the admin RestService.
func NewAdminRestService(next CompanyAdminService) *AdminRestService {
	return &AdminRestService{next: next}
}

// adminListResponse is the envelope for GET /admin/companies. Extends the
// standard paged envelope with total_active for header stats (counts every
// company_scan_settings row with active=true).
type adminListResponse struct {
	Data        []companydto.CompanyAdminView `json:"data"`
	Total       int64                         `json:"total"`
	TotalActive int64                         `json:"total_active"`
	Page        int                           `json:"page"`
	Limit       int                           `json:"limit"`
}

// list handles GET /admin/companies.
func (h *AdminRestService) list(w http.ResponseWriter, r *http.Request) {
	p := request.ParsePagination(r)
	q := r.URL.Query()

	var filter companydto.CompanyFilter
	if types := q["adapter_types"]; len(types) > 0 {
		filter.AdapterTypes = types
	} else if v := q.Get("adapter_type"); v != "" {
		filter.AdapterTypes = []string{v}
	}
	if v := q.Get("active"); v != "" {
		active := v == "true"
		filter.Active = &active
	}
	if v := q.Get("has_settings"); v != "" {
		hs := v == "true"
		filter.HasSettings = &hs
	}
	if v := q.Get("search"); v != "" {
		filter.Search = v
	}
	if v := q.Get("sort_by"); v != "" {
		filter.SortBy = v
	}
	if v := q.Get("sort_dir"); v != "" {
		filter.SortDir = v
	}

	result, err := h.next.List(r.Context(), filter, p.Page, p.Limit)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, adminListResponse{
		Data:        result.Items,
		Total:       result.Total,
		TotalActive: result.TotalActive,
		Page:        p.Page,
		Limit:       p.Limit,
	})
}

// get handles GET /admin/companies/{id}.
func (h *AdminRestService) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	view, err := h.next.Get(r.Context(), id)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, view)
}

// adminCreateRequest is the POST /admin/companies body envelope. Public
// fields and scan_settings travel together so admins can create + onboard
// in one round trip.
type adminCreateRequest struct {
	companydto.CompanyPublicInput
	ScanSettings *companydto.CompanyScanSettingsInput `json:"scan_settings"`
}

// create handles POST /admin/companies.
func (h *AdminRestService) create(w http.ResponseWriter, r *http.Request) {
	var body adminCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	settings := companydto.CompanyScanSettingsInput{}
	if body.ScanSettings != nil {
		settings = *body.ScanSettings
	}
	view, err := h.next.Create(r.Context(), body.CompanyPublicInput, settings)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusCreated, view)
}

// updatePublic handles PATCH /admin/companies/{id}.
func (h *AdminRestService) updatePublic(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var input companydto.CompanyPublicInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if err := h.next.UpdatePublic(r.Context(), id, input); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]string{"id": id})
}

// updateScanSettings handles PATCH /admin/companies/{id}/scan-settings.
func (h *AdminRestService) updateScanSettings(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var input companydto.CompanyScanSettingsInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if err := h.next.UpdateScanSettings(r.Context(), id, input); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]string{"id": id})
}

// delete handles DELETE /admin/companies/{id}.
func (h *AdminRestService) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.next.Delete(r.Context(), id); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]string{"id": id})
}

// setAllActive handles PATCH /admin/companies/active.
func (h *AdminRestService) setAllActive(w http.ResponseWriter, r *http.Request) {
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
