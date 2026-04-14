package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/store"
)

type CompanyHandler struct {
	companies           store.CompanyStore
	scanLog             store.CompanyScanLogReader
	queue               store.ScanQueueStore
	scanIntervalMinutes int
}

func NewCompanyHandler(companies store.CompanyStore, scanLog store.CompanyScanLogReader, queue store.ScanQueueStore, scanIntervalMinutes int) *CompanyHandler {
	return &CompanyHandler{companies: companies, scanLog: scanLog, queue: queue, scanIntervalMinutes: scanIntervalMinutes}
}

// Register mounts all company routes onto r.
func (h *CompanyHandler) Register(r chi.Router) {
	r.Route("/companies", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Patch("/active", h.SetAllActive)

		// Queue management — literal "queue" before {id} wildcard
		r.Get("/queue", h.ListQueue)
		r.Delete("/queue/{entry_id}", h.RemoveFromQueue)
		r.Post("/queue/{entry_id}/move-up", h.MoveQueueItemUp)
		r.Post("/queue/{entry_id}/move-down", h.MoveQueueItemDown)

		// Per-company routes
		r.Patch("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)
		r.Get("/{id}/scan-stats", h.ScanStats)
		r.Post("/{id}/scan", h.TriggerScan)
	})
}

// GET /companies
// Returns companies enriched with next_scan_at = last_scan_at + scan_interval.
func (h *CompanyHandler) List(w http.ResponseWriter, r *http.Request) {
	companies, err := h.companies.GetAllCompanies()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch companies")
		return
	}

	type CompanyView struct {
		model.Company
		NextScanAt *time.Time `json:"next_scan_at"`
	}

	views := make([]CompanyView, len(companies))
	for i, c := range companies {
		v := CompanyView{Company: c}
		if c.LastScanAt != nil {
			t := c.LastScanAt.Add(time.Duration(h.scanIntervalMinutes) * time.Minute)
			v.NextScanAt = &t
		}
		views[i] = v
	}

	respond(w, http.StatusOK, views)
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
	// Fixed-URL adapters have a built-in target — URL is optional for them.
	fixedURLAdapters := map[string]bool{
		model.AdapterGoogle:    true,
		model.AdapterNetflix:   true,
		model.AdapterUber:      true,
		model.AdapterApple:     true,
		model.AdapterMeta:      true,
		model.AdapterMicrosoft: true,
		model.AdapterRippling:  true,
	}
	if body.ID == "" || body.Name == "" || (!fixedURLAdapters[body.AdapterType] && body.URL == "") {
		respondError(w, http.StatusBadRequest, "id, name, and url are required")
		return
	}

	validAdapters := map[string]bool{
		model.AdapterGreenhouse: true,
		model.AdapterLever:      true,
		model.AdapterAshby:      true,
		model.AdapterGoogle:     true,
		model.AdapterNetflix:    true,
		model.AdapterUber:       true,
		model.AdapterApple:      true,
		model.AdapterMeta:       true,
		model.AdapterMicrosoft:  true,
		model.AdapterRippling:   true,
	}
	if !validAdapters[body.AdapterType] {
		respondError(w, http.StatusBadRequest, "invalid adapter_type")
		return
	}

	active := true
	if body.Active != nil {
		active = *body.Active
	}

	// Set last_scan_at to now so the scheduler doesn't immediately pick it up.
	// Users trigger the first scan explicitly via POST /companies/:id/scan.
	now := time.Now()
	company := model.Company{
		ID:          body.ID,
		Name:        body.Name,
		URL:         body.URL,
		AdapterType: body.AdapterType,
		Active:      active,
		LastScanAt:  &now,
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
			model.AdapterGoogle:     true,
			model.AdapterNetflix:    true,
			model.AdapterUber:       true,
		}
		if !validAdapters[at] {
			respondError(w, http.StatusBadRequest, "invalid adapter_type")
			return
		}
	}

	if err := h.companies.UpdateCompany(id, fields); err != nil {
		if store.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "company not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to update company")
		return
	}
	respond(w, http.StatusOK, map[string]string{"id": id})
}

// PATCH /companies/active
// Body: {"active": true|false} — sets all companies to the given active state.
func (h *CompanyHandler) SetAllActive(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Active *bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Active == nil {
		respondError(w, http.StatusBadRequest, "active (bool) is required")
		return
	}
	if err := h.companies.SetAllCompaniesActive(*body.Active); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update companies")
		return
	}
	respond(w, http.StatusOK, map[string]bool{"active": *body.Active})
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
// Enqueues a manual scan request. Always honours the request — no cooldown on manual triggers.
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

	if err := h.queue.Enqueue(id, model.QueueSourceManual); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to trigger scan")
		return
	}
	respond(w, http.StatusAccepted, map[string]string{"company_id": id, "status": "scan_queued"})
}

// GET /companies/queue
// Returns enriched queue entries joined with company name, adapter, and next_scan_at.
func (h *CompanyHandler) ListQueue(w http.ResponseWriter, r *http.Request) {
	items, err := h.queue.ListQueue()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch queue")
		return
	}

	allCompanies, err := h.companies.GetAllCompanies()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch companies")
		return
	}

	companyMap := make(map[string]model.Company, len(allCompanies))
	for _, c := range allCompanies {
		companyMap[c.ID] = c
	}

	type QueueEntryView struct {
		ID          string     `json:"id"`
		CompanyID   string     `json:"company_id"`
		CompanyName string     `json:"company_name"`
		AdapterType string     `json:"adapter_type"`
		Status      string     `json:"status"`
		Source      string     `json:"source"`
		Priority    int        `json:"priority"`
		QueuedAt    time.Time  `json:"queued_at"`
		StartedAt   *time.Time `json:"started_at"`
		NextScanAt  *time.Time `json:"next_scan_at"`
	}

	views := make([]QueueEntryView, 0, len(items))
	for _, item := range items {
		v := QueueEntryView{
			ID:        item.ID,
			CompanyID: item.CompanyID,
			Status:    item.Status,
			Source:    item.Source,
			Priority:  item.Priority,
			QueuedAt:  item.QueuedAt,
			StartedAt: item.StartedAt,
		}
		if c, ok := companyMap[item.CompanyID]; ok {
			v.CompanyName = c.Name
			v.AdapterType = c.AdapterType
			if c.LastScanAt != nil {
				t := c.LastScanAt.Add(time.Duration(h.scanIntervalMinutes) * time.Minute)
				v.NextScanAt = &t
			}
		}
		views = append(views, v)
	}

	respond(w, http.StatusOK, views)
}

// DELETE /companies/queue/:entry_id
// Removes a queued entry. No-ops if the entry is processing.
func (h *CompanyHandler) RemoveFromQueue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "entry_id")
	if err := h.queue.Remove(id); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to remove queue entry")
		return
	}
	respond(w, http.StatusOK, map[string]string{"id": id})
}

// POST /companies/queue/:entry_id/move-up
func (h *CompanyHandler) MoveQueueItemUp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "entry_id")
	if err := h.queue.MoveUp(id); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to move queue entry")
		return
	}
	respond(w, http.StatusOK, map[string]string{"id": id})
}

// POST /companies/queue/:entry_id/move-down
func (h *CompanyHandler) MoveQueueItemDown(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "entry_id")
	if err := h.queue.MoveDown(id); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to move queue entry")
		return
	}
	respond(w, http.StatusOK, map[string]string{"id": id})
}
