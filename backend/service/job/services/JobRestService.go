package services

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
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

// listInput is the Huma input for GET /api/jobs. Every field is read as a
// raw string (or, for repeated params, []string with the "explode" tag
// modifier so repeated ?key=a&key=b query keys are collected — Huma's
// default array style is comma-separated, which would silently break every
// existing frontend URL) and resolved manually in Resolve, replicating the
// legacy handler's exact silent-fallback/whitelist/coercion behavior. Using
// native typed (int/bool) fields would make Huma reject values the legacy
// handler tolerates (e.g. a non-clamped over-limit value forwarded to the
// validator, or a non-"true"/"false" boolean token) with a 400 instead of
// silently coercing or dropping — a real behavior change this migration
// must not introduce.
type listInput struct {
	Status           string   `query:"status"`
	Statuses         []string `query:"statuses,explode"`
	Companies        []string `query:"companies,explode"`
	EmployTypes      []string `query:"employ_types,explode"`
	ExperienceRanges []string `query:"experience_ranges,explode"`
	Search           string   `query:"search"`
	Remote           string   `query:"remote"`
	Sponsorship      string   `query:"sponsorship"`
	RequiresCitizen  string   `query:"requires_citizen"`
	OnSite           string   `query:"on_site"`
	Hybrid           string   `query:"hybrid"`
	NoRequirements   string   `query:"no_requirements"`
	DaysAdded        string   `query:"days_added"`
	SortBy           string   `query:"sort_by" doc:"title, company, added_at, or experience_years; unrecognized values fall back to the default"`
	SortDir          string   `query:"sort_dir" doc:"asc or desc"`
	Cursor           string   `query:"cursor"`
	Limit            string   `query:"limit" doc:"Page size; missing/invalid/non-positive values default to 20 (no upper clamp here — the validator enforces the cap)" example:"20"`

	filter dto.JobFilter
	limit  int
}

// Resolve replicates JobRestService.list's exact pre-Huma parsing.
func (in *listInput) Resolve(_ huma.Context) []error {
	limit := 20
	if n, err := strconv.Atoi(in.Limit); err == nil && n > 0 {
		limit = n
	}
	in.limit = limit

	var expRanges []dto.ExperienceRange
	for _, raw := range in.ExperienceRanges {
		if rng, ok := parseExperienceRange(raw); ok {
			expRanges = append(expRanges, rng)
		}
	}

	filter := dto.JobFilter{
		Status:           in.Status,
		Statuses:         in.Statuses,
		Companies:        in.Companies,
		EmployTypes:      in.EmployTypes,
		ExperienceRanges: expRanges,
		Search:           in.Search,
	}
	if in.Remote != "" {
		b := in.Remote == "true"
		filter.Remote = &b
	}
	if in.Sponsorship != "" {
		b := in.Sponsorship == "true"
		filter.Sponsorship = &b
	}
	if in.RequiresCitizen != "" {
		b := in.RequiresCitizen == "true"
		filter.RequiresCitizen = &b
	}
	if in.OnSite == "true" {
		filter.OnSite = true
	}
	if in.Hybrid == "true" {
		filter.Hybrid = true
	}
	if in.NoRequirements == "true" {
		filter.NoRequirements = true
	}
	if n, err := strconv.Atoi(in.DaysAdded); err == nil && n > 0 {
		filter.DaysAdded = &n
	}
	// Whitelist sort_by values to prevent raw column injection.
	if v := in.SortBy; v == "title" || v == "company" || v == "added_at" || v == "experience_years" {
		filter.SortBy = v
	}
	if v := in.SortDir; v == "asc" || v == "desc" {
		filter.SortDir = v
	}
	in.filter = filter
	return nil
}

// list serves GET /api/jobs with keyset pagination via ?cursor=... .
func (h *RestService) list(ctx context.Context, in *listInput) (*apihuma.SingleOutput[dto.PageResult], error) {
	page, err := h.next.List(ctx, in.filter, in.Cursor, in.limit)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[dto.PageResult]{}
	out.Body.Data = page
	return out, nil
}

// companies serves GET /api/jobs/companies — distinct company names in the jobs table.
func (h *RestService) companies(ctx context.Context, _ *struct{}) (*apihuma.SingleOutput[[]string], error) {
	names, err := h.next.GetCompanies(ctx)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[[]string]{}
	out.Body.Data = names
	return out, nil
}

// counts serves GET /api/jobs/counts.
func (h *RestService) counts(ctx context.Context, _ *struct{}) (*apihuma.SingleOutput[dto.Counts], error) {
	c, err := h.next.GetCounts(ctx)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[dto.Counts]{}
	out.Body.Data = c
	return out, nil
}

// getInput is the Huma input for GET /api/jobs/{id}.
type getInput struct {
	ID string `path:"id"`
}

// get serves GET /api/jobs/{id}.
func (h *RestService) get(ctx context.Context, in *getInput) (*apihuma.SingleOutput[dto.JobView], error) {
	job, err := h.next.Get(ctx, in.ID)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[dto.JobView]{}
	out.Body.Data = *job
	return out, nil
}

// setSavedInput is the Huma input for PATCH /api/jobs/{id}/save.
type setSavedInput struct {
	ID   string `path:"id"`
	Body struct {
		// required:"false" — an absent "saved" field defaults to false,
		// matching the pre-Huma json.Decode-into-zero-value behavior.
		Saved bool `json:"saved" required:"false"`
	}
}

// setSaved serves PATCH /api/jobs/{id}/save.
func (h *RestService) setSaved(ctx context.Context, in *setSavedInput) (*apihuma.SingleOutput[map[string]bool], error) {
	if err := h.next.SetSaved(ctx, in.ID, in.Body.Saved); err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[map[string]bool]{}
	out.Body.Data = map[string]bool{"saved": in.Body.Saved}
	return out, nil
}

// setStatusInput is the Huma input for PATCH /api/jobs/{id}/status.
type setStatusInput struct {
	ID   string `path:"id"`
	Body dto.SetStatusInput
}

// setStatus serves PATCH /api/jobs/{id}/status.
func (h *RestService) setStatus(ctx context.Context, in *setStatusInput) (*apihuma.SingleOutput[map[string]string], error) {
	if err := h.next.SetStatus(ctx, in.ID, in.Body.Status); err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[map[string]string]{}
	out.Body.Data = map[string]string{"status": in.Body.Status}
	return out, nil
}

// setNotesInput is the Huma input for PATCH /api/jobs/{id}/notes.
type setNotesInput struct {
	ID   string `path:"id"`
	Body dto.SetNotesInput
}

// setNotes serves PATCH /api/jobs/{id}/notes.
func (h *RestService) setNotes(ctx context.Context, in *setNotesInput) (*apihuma.SingleOutput[map[string]string], error) {
	if err := h.next.SetNotes(ctx, in.ID, in.Body.Notes); err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[map[string]string]{}
	out.Body.Data = map[string]string{"notes": in.Body.Notes}
	return out, nil
}

// dismissInput is the Huma input for DELETE /api/jobs/{id}. Body parsing is
// done by hand in Resolve, bypassing both of Huma's built-in body
// mechanisms: a typed Body field 400s on malformed JSON before this handler
// ever runs, and a RawBody field makes Huma unconditionally require a
// non-empty body (huma.go's initRequestBody(op, setRequestBodyRequired) has
// no per-field override) — but the legacy handler tolerates a MISSING or
// MALFORMED body, silently defaulting the dismissal reason in both cases.
// humachi.Unwrap gets the raw *http.Request so Resolve can replicate that
// exactly.
type dismissInput struct {
	ID string `path:"id"`

	reason string
	note   string
}

func (in *dismissInput) Resolve(ctx huma.Context) []error {
	r, _ := humachi.Unwrap(ctx)
	var body dto.DismissInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// Missing or malformed body — default reason kicks in, same as an
		// explicitly empty reason.
		body.Reason = jobcommon.DismissalReasonUserDismissed
	}
	if body.Reason == "" {
		body.Reason = jobcommon.DismissalReasonUserDismissed
	}
	in.reason = body.Reason
	in.note = body.Note
	return nil
}

// dismissOutput has no Body field, so Huma defaults its status to 204 (see
// huma's own op.DefaultStatus inference: no output body -> StatusNoContent).
type dismissOutput struct{}

// dismiss serves DELETE /api/jobs/{id}.
func (h *RestService) dismiss(ctx context.Context, in *dismissInput) (*dismissOutput, error) {
	if err := h.next.Dismiss(ctx, in.ID, in.reason, in.note); err != nil {
		return nil, apihuma.MapError(err)
	}
	return &dismissOutput{}, nil
}

// bulkDismissInput is the Huma input for POST /api/jobs/dismiss/bulk.
type bulkDismissInput struct {
	Body dto.BulkDismissInput
}

// bulkDismiss serves POST /api/jobs/dismiss/bulk.
func (h *RestService) bulkDismiss(ctx context.Context, in *bulkDismissInput) (*apihuma.SingleOutput[map[string]int], error) {
	reason := in.Body.Reason
	if reason == "" {
		reason = jobcommon.DismissalReasonNotRelevant
	}
	if err := h.next.BulkDismiss(ctx, in.Body.IDs, reason, in.Body.Note); err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[map[string]int]{}
	out.Body.Data = map[string]int{"dismissed": len(in.Body.IDs)}
	return out, nil
}

// parseExperienceRange parses "0-2" or "5+" into an ExperienceRange.
// Invalid strings return ok=false so callers can drop them silently.
func parseExperienceRange(s string) (dto.ExperienceRange, bool) {
	if rest, ok := strings.CutSuffix(s, "+"); ok {
		minVal, err := strconv.Atoi(rest)
		if err != nil || minVal < 0 {
			return dto.ExperienceRange{}, false
		}
		return dto.ExperienceRange{Min: minVal}, true
	}
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return dto.ExperienceRange{}, false
	}
	minVal, err1 := strconv.Atoi(parts[0])
	maxVal, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || minVal < 0 || maxVal <= minVal {
		return dto.ExperienceRange{}, false
	}
	return dto.ExperienceRange{Min: minVal, Max: &maxVal}, true
}
