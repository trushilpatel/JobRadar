package job

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/api/handler"
	"github.com/trushilpatel/jobradar/api/request"
	"github.com/trushilpatel/jobradar/model"
)

// Handler handles HTTP requests for the job domain.
type Handler struct {
	validator Validator
	jobs      Service
}

// NewHandler constructs a Handler.
func NewHandler(v Validator, svc Service) *Handler {
	return &Handler{validator: v, jobs: svc}
}

// Register mounts all job routes onto r.
func (h *Handler) Register(r chi.Router) {
	r.Get("/jobs", h.List)
	r.Get("/jobs/counts", h.GetCounts)
	r.Get("/jobs/{id}", h.Get)
	r.Delete("/jobs/{id}", h.Dismiss)
	r.Patch("/jobs/{id}/save", h.SetSaved)
	r.Patch("/jobs/{id}/status", h.SetStatus)
	r.Patch("/jobs/{id}/notes", h.SetNotes)
}

// GET /jobs?status=new&page=1&limit=20&remote=true&sponsorship=true&...
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	p := request.ParsePagination(r)
	q := r.URL.Query()

	filter := JobFilter{
		Status:           q.Get("status"),
		Companies:        q["companies"],         // ?companies=stripe&companies=google
		EmployTypes:      q["employ_types"],      // ?employ_types=full_time
		ExperienceRanges: q["experience_ranges"], // ?experience_ranges=0-5&experience_ranges=5%2B
	}

	if filter.Status == "" {
		filter.Status = model.JobStatusNew
	}

	if v := q.Get("remote"); v == "true" {
		t := true
		filter.Remote = &t
	}
	if v := q.Get("sponsorship"); v != "" {
		b := v == "true"
		filter.Sponsorship = &b
	}
	if q.Get("no_requirements") == "true" {
		filter.NoRequirements = true
	}
	if v := q.Get("requires_citizen"); v != "" {
		b := v == "true"
		filter.RequiresCitizen = &b
	}
	if q.Get("on_site") == "true" {
		filter.OnSite = true
	}
	if v := q.Get("days_added"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.DaysAdded = &n
		}
	}

	if err := h.validator.ValidateList(filter, p.Page, p.Limit); err != nil {
		handler.HandleError(w, err)
		return
	}

	jobs, total, err := h.jobs.List(r.Context(), filter, p.Page, p.Limit)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.RespondPaged(w, http.StatusOK, jobs, total, p.Page, p.Limit)
}

// GET /jobs/counts
func (h *Handler) GetCounts(w http.ResponseWriter, r *http.Request) {
	counts, err := h.jobs.GetCounts(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, counts)
}

// GET /jobs/:id
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	job, err := h.jobs.Get(r.Context(), id)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, job)
}

// DELETE /jobs/:id
func (h *Handler) Dismiss(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input DismissInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.validator.ValidateDismiss(input); err != nil {
		handler.HandleError(w, err)
		return
	}

	if err := h.jobs.Dismiss(r.Context(), id, input); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"id": id, "reason": input.Reason})
}

// PATCH /jobs/:id/save
func (h *Handler) SetSaved(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Saved bool `json:"saved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.jobs.SetSaved(r.Context(), id, body.Saved); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]bool{"saved": body.Saved})
}

// PATCH /jobs/:id/status
func (h *Handler) SetStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input SetStatusInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.validator.ValidateSetStatus(input.Status); err != nil {
		handler.HandleError(w, err)
		return
	}

	if err := h.jobs.SetStatus(r.Context(), id, input.Status); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"status": input.Status})
}

// PATCH /jobs/:id/notes
func (h *Handler) SetNotes(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input SetNotesInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.validator.ValidateSetNotes(input); err != nil {
		handler.HandleError(w, err)
		return
	}

	if err := h.jobs.SetNotes(r.Context(), id, input.Notes); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"notes": input.Notes})
}
