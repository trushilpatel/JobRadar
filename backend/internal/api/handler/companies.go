package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
)

type CompanyHandler struct {
	companies store.CompanyStore
	scanLog   store.JobScanLogReader
	publisher kafka.Publisher
}

func NewCompanyHandler(companies store.CompanyStore, scanLog store.JobScanLogReader, publisher kafka.Publisher) *CompanyHandler {
	return &CompanyHandler{companies: companies, scanLog: scanLog, publisher: publisher}
}

// GET /companies
func (h *CompanyHandler) List(w http.ResponseWriter, r *http.Request) {
	companies, err := h.companies.GetAllCompanies()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch companies")
		return
	}
	respond(w, http.StatusOK, companies)
}

// POST /companies
// Body: {"id":"stripe","name":"Stripe","url":"...","adapter_type":"greenhouse","active":true}
func (h *CompanyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		URL         string `json:"url"`
		AdapterType string `json:"adapter_type"`
		Active      *bool  `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.ID == "" || body.Name == "" || body.URL == "" {
		respondError(w, http.StatusBadRequest, "id, name, and url are required")
		return
	}

	validAdapters := map[string]bool{
		model.AdapterGreenhouse: true,
		model.AdapterLever:      true,
		model.AdapterAshby:      true,
		model.AdapterWorkday:    true,
		model.AdapterWorkable:   true,
		model.AdapterChromedp:   true,
	}
	if !validAdapters[body.AdapterType] {
		respondError(w, http.StatusBadRequest, "invalid adapter_type")
		return
	}

	active := true
	if body.Active != nil {
		active = *body.Active
	}

	company := model.Company{
		ID:          body.ID,
		Name:        body.Name,
		URL:         body.URL,
		AdapterType: body.AdapterType,
		Active:      active,
	}
	if err := h.companies.CreateCompany(company); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create company")
		return
	}
	respond(w, http.StatusCreated, company)
}

// PATCH /companies/:id
// Body: any subset of {name, url, adapter_type, active}
func (h *CompanyHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	allowed := map[string]bool{"name": true, "url": true, "adapter_type": true, "active": true}
	fields := make(map[string]any, len(body))
	for k, v := range body {
		if allowed[k] {
			fields[k] = v
		}
	}
	if len(fields) == 0 {
		respondError(w, http.StatusBadRequest, "no valid fields to update")
		return
	}

	if at, ok := fields["adapter_type"].(string); ok {
		validAdapters := map[string]bool{
			model.AdapterGreenhouse: true,
			model.AdapterLever:      true,
			model.AdapterAshby:      true,
			model.AdapterWorkday:    true,
			model.AdapterWorkable:   true,
			model.AdapterChromedp:   true,
		}
		if !validAdapters[at] {
			respondError(w, http.StatusBadRequest, "invalid adapter_type")
			return
		}
	}

	if err := h.companies.UpdateCompany(id, fields); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update company")
		return
	}
	respond(w, http.StatusOK, map[string]string{"id": id})
}

// DELETE /companies/:id
func (h *CompanyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if _, err := h.companies.GetCompanyByID(id); err != nil {
		if store.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "company not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch company")
		return
	}

	if err := h.companies.DeleteCompany(id); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete company")
		return
	}
	respond(w, http.StatusOK, map[string]string{"id": id})
}

// GET /companies/:id/scan-stats
func (h *CompanyHandler) ScanStats(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	stats, err := h.scanLog.GetScanStats(id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch scan stats")
		return
	}
	respond(w, http.StatusOK, stats)
}

// POST /companies/:id/scan
// Publishes a scan request to company.scan_job — scan worker picks it up async.
func (h *CompanyHandler) TriggerScan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if _, err := h.companies.GetCompanyByID(id); err != nil {
		if store.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "company not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch company")
		return
	}

	payload, _ := json.Marshal(map[string]string{"company_id": id})
	if err := h.publisher.Publish(context.Background(), kafka.TopicCompanyScanJob, payload); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to trigger scan")
		return
	}
	respond(w, http.StatusAccepted, map[string]string{"company_id": id, "status": "scan_queued"})
}
