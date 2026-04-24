package services

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/apps/api/request"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
)

// RestService handles HTTP requests for the job domain.
// It parses requests, calls the service chain, and writes responses.
// Routing belongs in RestRouter.
type RestService struct {
	next Service
}

// NewRestService constructs a RestService.
func NewRestService(next Service) *RestService {
	return &RestService{next: next}
}

// GET /jobs?status=new&page=1&limit=20&remote=true&sponsorship=true&...
func (h *RestService) List(w http.ResponseWriter, r *http.Request) {
	p := request.ParsePagination(r)
	q := r.URL.Query()

	var expRanges []dto.ExperienceRange
	for _, raw := range q["experience_ranges"] {
		if r, ok := parseExperienceRange(raw); ok {
			expRanges = append(expRanges, r)
		}
	}

	filter := dto.JobFilter{
		Status:           q.Get("status"),
		Companies:        q["companies"],    // ?companies=stripe&companies=google
		EmployTypes:      q["employ_types"], // ?employ_types=full_time
		ExperienceRanges: expRanges,
	}

	if filter.Status == "" {
		filter.Status = jobcommon.JobStatusNew
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
	if v := q.Get("sort_by"); v != "" {
		filter.SortBy = v
	}
	if v := q.Get("sort_dir"); v != "" {
		filter.SortDir = v
	}

	jobs, total, err := h.next.List(r.Context(), filter, p.Page, p.Limit)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.RespondPaged(w, http.StatusOK, jobs, total, p.Page, p.Limit)
}

// GET /jobs/counts
func (h *RestService) GetCounts(w http.ResponseWriter, r *http.Request) {
	counts, err := h.next.GetCounts(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, counts)
}

// GET /jobs/:id
func (h *RestService) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	job, err := h.next.Get(r.Context(), id)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, job)
}

// DELETE /jobs/:id
func (h *RestService) Dismiss(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input dto.DismissInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.next.Dismiss(r.Context(), id, input); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"id": id, "reason": input.Reason})
}

// PATCH /jobs/:id/save
func (h *RestService) SetSaved(w http.ResponseWriter, r *http.Request) {
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

// PATCH /jobs/:id/status
func (h *RestService) SetStatus(w http.ResponseWriter, r *http.Request) {
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

// PATCH /jobs/:id/notes
func (h *RestService) SetNotes(w http.ResponseWriter, r *http.Request) {
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
