package services

import (
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

// adminListEnvelope is the JSON envelope for GET /admin/companies. Extends
// the cursor-paged envelope with total_active for header stats.
// Uses CompanyAdminListView (no adapter_config) — the detail endpoint
// (GET /admin/companies/{id}) returns the full CompanyAdminView with adapter_config.
type adminListEnvelope struct {
	Data        []companydto.CompanyAdminListView `json:"data"`
	Total       int64                             `json:"total"`
	TotalActive int64                             `json:"total_active"`
	Limit       int                               `json:"limit"`
	NextCursor  *string                           `json:"next_cursor"`
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
	if v := q.Get("scan_status"); v != "" {
		filter.ScanStatus = v
	}
	if v := q.Get("cursor"); v != "" {
		filter.Cursor = v
	}

	result, err := h.next.List(r.Context(), filter, p.Limit)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, adminListEnvelope{
		Data:        result.Items,
		Total:       result.Total,
		TotalActive: result.TotalActive,
		Limit:       p.Limit,
		NextCursor:  result.NextCursor,
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

// setAllActiveRequest is the PATCH /admin/companies/active body envelope.
type setAllActiveRequest struct {
	Active *bool `json:"active"`
}

// create handles POST /admin/companies.
func (h *AdminRestService) create(w http.ResponseWriter, r *http.Request) {
	body, ok := handler.DecodeBody[adminCreateRequest](w, r)
	if !ok {
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
	input, ok := handler.DecodeBody[companydto.CompanyPublicInput](w, r)
	if !ok {
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
	input, ok := handler.DecodeBody[companydto.CompanyScanSettingsInput](w, r)
	if !ok {
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
	w.WriteHeader(http.StatusNoContent)
}

// setAllActive handles PATCH /admin/companies/active.
func (h *AdminRestService) setAllActive(w http.ResponseWriter, r *http.Request) {
	body, ok := handler.DecodeBody[setAllActiveRequest](w, r)
	if !ok {
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
