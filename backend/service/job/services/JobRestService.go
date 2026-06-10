package services

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
)

// RestService is the HTTP adapter for the JobService chain. It parses
// requests, calls the service chain (which pulls userID from ctx via
// middleware), and writes JSON responses via the shared handler helpers.
type RestService struct {
	next JobService
}

// NewRestService constructs a RestService.
func NewRestService(next JobService) *RestService {
	return &RestService{next: next}
}

// list serves GET /api/jobs with keyset pagination via ?cursor=... .
// Limit is clamped to [1,100]; defaults to 20. Filter parsing mirrors the
// previous offset-paginated handler so existing query strings remain
// compatible (except `page=` is now ignored).
func (h *RestService) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit := 20
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	var expRanges []dto.ExperienceRange
	for _, raw := range q["experience_ranges"] {
		if rng, ok := parseExperienceRange(raw); ok {
			expRanges = append(expRanges, rng)
		}
	}

	filter := dto.JobFilter{
		Status:           q.Get("status"),
		Companies:        q["companies"],
		EmployTypes:      q["employ_types"],
		ExperienceRanges: expRanges,
		Search:           q.Get("search"),
	}
	if v := q.Get("remote"); v != "" {
		b := v == "true"
		filter.Remote = &b
	}
	if v := q.Get("sponsorship"); v != "" {
		b := v == "true"
		filter.Sponsorship = &b
	}
	if v := q.Get("requires_citizen"); v != "" {
		b := v == "true"
		filter.RequiresCitizen = &b
	}
	if q.Get("on_site") == "true" {
		filter.OnSite = true
	}
	if q.Get("hybrid") == "true" {
		filter.Hybrid = true
	}
	if q.Get("no_requirements") == "true" {
		filter.NoRequirements = true
	}
	if v := q.Get("days_added"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.DaysAdded = &n
		}
	}
	// Whitelist sort_by values to prevent raw column injection.
	if v := q.Get("sort_by"); v == "title" || v == "company" || v == "added_at" || v == "experience_years" {
		filter.SortBy = v
	}
	if v := q.Get("sort_dir"); v == "asc" || v == "desc" {
		filter.SortDir = v
	}

	page, err := h.next.List(r.Context(), filter, q.Get("cursor"), limit)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, page)
}

// companies serves GET /api/jobs/companies — distinct company names in the jobs table.
func (h *RestService) companies(w http.ResponseWriter, r *http.Request) {
	names, err := h.next.GetCompanies(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, names)
}

// counts serves GET /api/jobs/counts.
func (h *RestService) counts(w http.ResponseWriter, r *http.Request) {
	c, err := h.next.GetCounts(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, c)
}

// get serves GET /api/jobs/{id}.
func (h *RestService) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := h.next.Get(r.Context(), id)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, job)
}

// setSaved serves PATCH /api/jobs/{id}/save.
func (h *RestService) setSaved(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Saved bool `json:"saved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if err := h.next.SetSaved(r.Context(), id, body.Saved); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]bool{"saved": body.Saved})
}

// setStatus serves PATCH /api/jobs/{id}/status.
func (h *RestService) setStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var input dto.SetStatusInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if err := h.next.SetStatus(r.Context(), id, input.Status); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]string{"status": input.Status})
}

// setNotes serves PATCH /api/jobs/{id}/notes.
func (h *RestService) setNotes(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var input dto.SetNotesInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if err := h.next.SetNotes(r.Context(), id, input.Notes); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]string{"notes": input.Notes})
}

// dismiss serves DELETE /api/jobs/{id}.
func (h *RestService) dismiss(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var input dto.DismissInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		// DELETE may have empty body — default reason kicks in.
		input.Reason = jobcommon.DismissalReasonUserDismissed
	}
	if input.Reason == "" {
		input.Reason = jobcommon.DismissalReasonUserDismissed
	}
	if err := h.next.Dismiss(r.Context(), id, input.Reason, input.Note); err != nil {
		handler.HandleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// bulkDismiss serves POST /api/jobs/dismiss/bulk.
func (h *RestService) bulkDismiss(w http.ResponseWriter, r *http.Request) {
	var input dto.BulkDismissInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if input.Reason == "" {
		input.Reason = jobcommon.DismissalReasonNotRelevant
	}
	if err := h.next.BulkDismiss(r.Context(), input.IDs, input.Reason, input.Note); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]int{"dismissed": len(input.IDs)})
}

// parseExperienceRange parses "0-2" or "5+" into an ExperienceRange.
// Invalid strings return ok=false so callers can drop them silently.
func parseExperienceRange(s string) (dto.ExperienceRange, bool) {
	if rest, ok := strings.CutSuffix(s, "+"); ok {
		min, err := strconv.Atoi(rest)
		if err != nil || min < 0 {
			return dto.ExperienceRange{}, false
		}
		return dto.ExperienceRange{Min: min}, true
	}
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return dto.ExperienceRange{}, false
	}
	min, err1 := strconv.Atoi(parts[0])
	max, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || min < 0 || max <= min {
		return dto.ExperienceRange{}, false
	}
	return dto.ExperienceRange{Min: min, Max: &max}, true
}
