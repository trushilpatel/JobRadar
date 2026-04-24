package services

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// AdapterConfigRestService handles HTTP requests for the adapter_configs domain.
// It has a single dependency on AdapterConfigService; routing belongs in AdapterConfigRestRouter.
type AdapterConfigRestService struct {
	next AdapterConfigService
}

// NewAdapterConfigRestService constructs an AdapterConfigRestService.
func NewAdapterConfigRestService(next AdapterConfigService) *AdapterConfigRestService {
	return &AdapterConfigRestService{next: next}
}

// GET /adapter-configs
func (h *AdapterConfigRestService) ListAdapterConfigs(w http.ResponseWriter, r *http.Request) {
	configs, err := h.next.ListAdapterConfigs(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, configs)
}

// PATCH /adapter-configs/{adapter_type}
func (h *AdapterConfigRestService) UpdateAdapterConfig(w http.ResponseWriter, r *http.Request) {
	adapterType := chi.URLParam(r, "adapter_type")

	var input dto.UpdateAdapterConfigInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	updated, err := h.next.UpdateAdapterConfig(r.Context(), adapterType, input)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, updated)
}

// PATCH /companies/{id}/scan-interval-override
func (h *AdapterConfigRestService) UpdateScanIntervalOverride(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "id")

	var body struct {
		ScanIntervalMinutesOverride *int `json:"scan_interval_minutes_override"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.next.UpdateScanIntervalOverride(r.Context(), companyID, body.ScanIntervalMinutesOverride); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]string{"company_id": companyID, "status": "updated"})
}

// GET /companies/{id}/scan-config
func (h *AdapterConfigRestService) GetEffectiveScanConfig(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "id")

	config, err := h.next.GetEffectiveScanConfig(r.Context(), companyID)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, config)
}
