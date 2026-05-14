package services_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// fakeRestSvc is a JobService test double used to drive the REST layer.
// Captures inputs so handler-level wiring (filter parsing, body decoding)
// can be asserted without spinning up the impl chain.
type fakeRestSvc struct {
	listResult jobdto.PageResult
	listErr    error
	lastFilter jobdto.JobFilter
	lastCursor string
	lastLimit  int

	getResult *jobdto.JobView
	getErr    error

	bulkErr    error
	bulkIDs    []string
	bulkReason string
	bulkNote   string

	setSavedID    string
	setSavedValue bool
	setSavedErr   error

	setStatusID    string
	setStatusValue string
	setStatusErr   error

	setNotesID    string
	setNotesValue string
	setNotesErr   error

	dismissID     string
	dismissReason string
	dismissNote   string
	dismissErr    error
}

func (f *fakeRestSvc) List(_ context.Context, filter jobdto.JobFilter, cursor string, limit int) (jobdto.PageResult, error) {
	f.lastFilter = filter
	f.lastCursor = cursor
	f.lastLimit = limit
	return f.listResult, f.listErr
}
func (f *fakeRestSvc) Get(_ context.Context, _ string) (*jobdto.JobView, error) {
	return f.getResult, f.getErr
}
func (f *fakeRestSvc) SetSaved(_ context.Context, id string, saved bool) error {
	f.setSavedID = id
	f.setSavedValue = saved
	return f.setSavedErr
}
func (f *fakeRestSvc) SetStatus(_ context.Context, id, status string) error {
	f.setStatusID = id
	f.setStatusValue = status
	return f.setStatusErr
}
func (f *fakeRestSvc) SetNotes(_ context.Context, id, notes string) error {
	f.setNotesID = id
	f.setNotesValue = notes
	return f.setNotesErr
}
func (f *fakeRestSvc) Dismiss(_ context.Context, id string, reason, note string) error {
	f.dismissID = id
	f.dismissReason = reason
	f.dismissNote = note
	return f.dismissErr
}
func (f *fakeRestSvc) BulkDismiss(_ context.Context, ids []string, reason, note string) error {
	f.bulkIDs = ids
	f.bulkReason = reason
	f.bulkNote = note
	return f.bulkErr
}
func (f *fakeRestSvc) GetCounts(_ context.Context) (jobdto.Counts, error) {
	return jobdto.Counts{}, nil
}
func (f *fakeRestSvc) GetCompanies(_ context.Context) ([]string, error) { return nil, nil }

var _ jobsvc.JobService = (*fakeRestSvc)(nil)

func newRouter(svc jobsvc.JobService) *chi.Mux {
	rest := jobsvc.NewRestService(svc)
	router := jobsvc.NewRestRouter(rest)
	r := chi.NewRouter()
	router.Register(r)
	return r
}

func TestRest_List_OK_HasNextCursor(t *testing.T) {
	r := newRouter(&fakeRestSvc{listResult: jobdto.PageResult{Items: []jobdto.JobView{}, NextCursor: "abc"}})
	req := httptest.NewRequest(http.MethodGet, "/jobs?cursor=xyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"next_cursor":"abc"`) {
		t.Errorf("response missing next_cursor: %s", w.Body.String())
	}
}

func TestRest_List_ForwardsCursor(t *testing.T) {
	fake := &fakeRestSvc{}
	r := newRouter(fake)
	req := httptest.NewRequest(http.MethodGet, "/jobs?cursor=opaque-cursor", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if fake.lastCursor != "opaque-cursor" {
		t.Errorf("cursor = %q, want %q", fake.lastCursor, "opaque-cursor")
	}
}

func TestRest_List_ServiceErrorMapsTo500(t *testing.T) {
	r := newRouter(&fakeRestSvc{listErr: apperr.ErrInternal.WithMessage("db down")})
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestRest_BulkDismiss_DefaultReason(t *testing.T) {
	fake := &fakeRestSvc{}
	r := newRouter(fake)
	body, _ := json.Marshal(map[string][]string{"ids": {"j1", "j2"}})
	req := httptest.NewRequest(http.MethodPost, "/jobs/dismiss/bulk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if fake.bulkReason != jobcommon.DismissalReasonNotRelevant {
		t.Errorf("default reason = %q, want %q", fake.bulkReason, jobcommon.DismissalReasonNotRelevant)
	}
	if len(fake.bulkIDs) != 2 {
		t.Errorf("ids = %v, want 2 entries", fake.bulkIDs)
	}
}

func TestRest_BulkDismiss_BadJSON(t *testing.T) {
	r := newRouter(&fakeRestSvc{})
	req := httptest.NewRequest(http.MethodPost, "/jobs/dismiss/bulk", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestRest_Get_NotFound(t *testing.T) {
	r := newRouter(&fakeRestSvc{getErr: apperr.ErrNotFound.WithMessage("missing")})
	req := httptest.NewRequest(http.MethodGet, "/jobs/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// -------- Comprehensive list-handler query-param parsing --------

// TestRest_List_ParsesAllFilters drives the handler with the full filter URL
// the frontend produces (every supported filter on at once) and asserts each
// field is parsed into the JobFilter correctly. Regressions in the parser
// silently turn into wrong results — the bug class that triggered this audit.
func TestRest_List_ParsesAllFilters(t *testing.T) {
	fake := &fakeRestSvc{}
	r := newRouter(fake)

	req := httptest.NewRequest(http.MethodGet,
		"/jobs?status=saved&search=engineer&limit=50&cursor=tok"+
			"&companies=Google&companies=Meta"+
			"&employ_types=full_time&employ_types=contract"+
			"&experience_ranges=0-2&experience_ranges=5%2B"+
			"&remote=true&sponsorship=false&requires_citizen=false"+
			"&on_site=true&no_requirements=true"+
			"&days_added=7&sort_by=title&sort_dir=asc",
		nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	got := fake.lastFilter
	if got.Status != "saved" {
		t.Errorf("status = %q, want saved", got.Status)
	}
	if got.Search != "engineer" {
		t.Errorf("search = %q, want engineer", got.Search)
	}
	if fake.lastLimit != 50 {
		t.Errorf("limit = %d, want 50", fake.lastLimit)
	}
	if fake.lastCursor != "tok" {
		t.Errorf("cursor = %q, want tok", fake.lastCursor)
	}
	if len(got.Companies) != 2 || got.Companies[0] != "Google" || got.Companies[1] != "Meta" {
		t.Errorf("companies = %v, want [Google Meta]", got.Companies)
	}
	if len(got.EmployTypes) != 2 {
		t.Errorf("employ_types len = %d, want 2", len(got.EmployTypes))
	}
	if len(got.ExperienceRanges) != 2 {
		t.Fatalf("experience_ranges len = %d, want 2", len(got.ExperienceRanges))
	}
	// 0-2 → Min=0 Max=2
	if got.ExperienceRanges[0].Min != 0 || got.ExperienceRanges[0].Max == nil || *got.ExperienceRanges[0].Max != 2 {
		t.Errorf("experience_ranges[0] = %+v, want {0,&2}", got.ExperienceRanges[0])
	}
	// 5+ → Min=5 Max=nil
	if got.ExperienceRanges[1].Min != 5 || got.ExperienceRanges[1].Max != nil {
		t.Errorf("experience_ranges[1] = %+v, want {5,nil}", got.ExperienceRanges[1])
	}
	if got.Remote == nil || *got.Remote != true {
		t.Errorf("remote = %v, want &true", got.Remote)
	}
	if got.Sponsorship == nil || *got.Sponsorship != false {
		t.Errorf("sponsorship = %v, want &false", got.Sponsorship)
	}
	if got.RequiresCitizen == nil || *got.RequiresCitizen != false {
		t.Errorf("requires_citizen = %v, want &false", got.RequiresCitizen)
	}
	if !got.OnSite {
		t.Errorf("on_site = %v, want true", got.OnSite)
	}
	if !got.NoRequirements {
		t.Errorf("no_requirements = %v, want true", got.NoRequirements)
	}
	if got.DaysAdded == nil || *got.DaysAdded != 7 {
		t.Errorf("days_added = %v, want &7", got.DaysAdded)
	}
	if got.SortBy != "title" {
		t.Errorf("sort_by = %q, want title", got.SortBy)
	}
	if got.SortDir != "asc" {
		t.Errorf("sort_dir = %q, want asc", got.SortDir)
	}
}

// TestRest_List_DefaultsWhenAbsent asserts the handler-applied defaults for an
// empty query string — limit=20, no filters, no cursor.
func TestRest_List_DefaultsWhenAbsent(t *testing.T) {
	fake := &fakeRestSvc{}
	r := newRouter(fake)
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if fake.lastLimit != 20 {
		t.Errorf("limit = %d, want 20 default", fake.lastLimit)
	}
	got := fake.lastFilter
	if got.Status != "" || got.Search != "" || len(got.Companies) != 0 ||
		got.Remote != nil || got.Sponsorship != nil || got.RequiresCitizen != nil ||
		got.OnSite || got.NoRequirements || got.DaysAdded != nil ||
		got.SortBy != "" || got.SortDir != "" || len(got.ExperienceRanges) != 0 {
		t.Errorf("non-zero default filter: %+v", got)
	}
}

// TestRest_List_BadLimit_FallsBackToDefault — non-numeric, zero, or negative
// limit silently falls back to 20; 200 (above the validator cap) is forwarded
// unchanged so the validation layer can report it. This pins the contract.
func TestRest_List_BadLimit_FallsBackToDefault(t *testing.T) {
	tests := []struct {
		raw      string
		wantSent int
	}{
		{raw: "abc", wantSent: 20},
		{raw: "0", wantSent: 20},
		{raw: "-1", wantSent: 20},
		{raw: "", wantSent: 20},
		{raw: "200", wantSent: 200}, // forwarded; validator rejects later
		{raw: "100", wantSent: 100}, // boundary
	}
	for _, tc := range tests {
		t.Run("limit="+tc.raw, func(t *testing.T) {
			fake := &fakeRestSvc{}
			r := newRouter(fake)
			req := httptest.NewRequest(http.MethodGet, "/jobs?limit="+tc.raw, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if fake.lastLimit != tc.wantSent {
				t.Errorf("limit forwarded = %d, want %d", fake.lastLimit, tc.wantSent)
			}
		})
	}
}

// TestRest_List_BadExperienceRange_SilentlyDropped — invalid experience_ranges
// entries are dropped without erroring the whole request. This is intentional:
// stale URLs from older UI builds shouldn't break the page.
func TestRest_List_BadExperienceRange_SilentlyDropped(t *testing.T) {
	fake := &fakeRestSvc{}
	r := newRouter(fake)
	// First range is valid; second two are malformed and should be dropped.
	req := httptest.NewRequest(http.MethodGet,
		"/jobs?experience_ranges=0-2&experience_ranges=garbage&experience_ranges=5-3", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(fake.lastFilter.ExperienceRanges) != 1 {
		t.Errorf("kept ranges = %d, want 1 (only 0-2 valid)", len(fake.lastFilter.ExperienceRanges))
	}
}

// TestRest_List_SortBy_Whitelisted ensures unrecognised sort_by values are
// silently dropped (NOT forwarded as a SQL injection vector).
func TestRest_List_SortBy_Whitelisted(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "title", want: "title"},
		{raw: "company", want: "company"},
		{raw: "added_at", want: "added_at"},
		{raw: "experience_years", want: "experience_years"},
		{raw: "id; DROP TABLE jobs", want: ""}, // injection attempt dropped
		{raw: "random_column", want: ""},
		{raw: "", want: ""},
	}
	for _, tc := range tests {
		t.Run("sort_by="+tc.raw, func(t *testing.T) {
			fake := &fakeRestSvc{}
			r := newRouter(fake)
			req := httptest.NewRequest(http.MethodGet, "/jobs?sort_by="+urlencode(tc.raw), nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if fake.lastFilter.SortBy != tc.want {
				t.Errorf("sort_by forwarded = %q, want %q", fake.lastFilter.SortBy, tc.want)
			}
		})
	}
}

// TestRest_List_SortDir_Whitelisted asserts only asc/desc are forwarded.
func TestRest_List_SortDir_Whitelisted(t *testing.T) {
	tests := []struct{ raw, want string }{
		{raw: "asc", want: "asc"},
		{raw: "desc", want: "desc"},
		{raw: "DESC", want: ""},
		{raw: "garbage", want: ""},
		{raw: "", want: ""},
	}
	for _, tc := range tests {
		t.Run("sort_dir="+tc.raw, func(t *testing.T) {
			fake := &fakeRestSvc{}
			r := newRouter(fake)
			req := httptest.NewRequest(http.MethodGet, "/jobs?sort_dir="+tc.raw, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if fake.lastFilter.SortDir != tc.want {
				t.Errorf("sort_dir forwarded = %q, want %q", fake.lastFilter.SortDir, tc.want)
			}
		})
	}
}

// TestRest_List_BadDaysAdded_Dropped asserts non-positive or non-numeric
// days_added is silently dropped (no filter applied).
func TestRest_List_BadDaysAdded_Dropped(t *testing.T) {
	tests := []string{"0", "-1", "abc", ""}
	for _, raw := range tests {
		t.Run("days_added="+raw, func(t *testing.T) {
			fake := &fakeRestSvc{}
			r := newRouter(fake)
			req := httptest.NewRequest(http.MethodGet, "/jobs?days_added="+raw, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if fake.lastFilter.DaysAdded != nil {
				t.Errorf("days_added = %v, want nil", fake.lastFilter.DaysAdded)
			}
		})
	}
}

// TestRest_List_Search_ForwardedVerbatim — search is passed through unmodified
// (no trimming, no wildcard escaping at the handler). Documents current
// behaviour so a fix is a deliberate change.
func TestRest_List_Search_ForwardedVerbatim(t *testing.T) {
	tests := []string{
		"engineer",
		"  whitespace  ", // current behaviour: NOT trimmed
		"%wildcards_",    // current behaviour: LIKE wildcards pass through
		"unicode 日本語",
		"O'Brien", // SQL-like quote, parameterised so safe at DB level
	}
	for _, raw := range tests {
		t.Run("search="+raw, func(t *testing.T) {
			fake := &fakeRestSvc{}
			r := newRouter(fake)
			// URL-encode the query manually to keep the test honest about what's sent.
			u := "/jobs?search=" + urlencode(raw)
			req := httptest.NewRequest(http.MethodGet, u, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if fake.lastFilter.Search != raw {
				t.Errorf("search forwarded = %q, want %q", fake.lastFilter.Search, raw)
			}
		})
	}
}

// urlencode is a tiny helper to avoid importing net/url just for the search
// edge-case table. Only encodes the characters that matter for the cases above.
func urlencode(s string) string {
	out := make([]byte, 0, len(s)*3)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			out = append(out, c)
		case c == ' ':
			out = append(out, '+')
		default:
			const hex = "0123456789ABCDEF"
			out = append(out, '%', hex[c>>4], hex[c&0x0F])
		}
	}
	return string(out)
}

// -------- Body decoding for write endpoints --------

func TestRest_SetSaved_DecodesBody(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    bool
		wantSts int
	}{
		{name: "saved true", body: `{"saved":true}`, want: true, wantSts: http.StatusOK},
		{name: "saved false", body: `{"saved":false}`, want: false, wantSts: http.StatusOK},
		{name: "missing field → default false", body: `{}`, want: false, wantSts: http.StatusOK},
		{name: "malformed JSON", body: `{not json`, wantSts: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRestSvc{}
			r := newRouter(fake)
			req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/save", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.wantSts {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.wantSts, w.Body.String())
			}
			if tc.wantSts == http.StatusOK {
				if fake.setSavedID != "j1" || fake.setSavedValue != tc.want {
					t.Errorf("forwarded (id=%q, saved=%v), want (j1, %v)",
						fake.setSavedID, fake.setSavedValue, tc.want)
				}
			}
		})
	}
}

func TestRest_SetStatus_DecodesBody(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantSts int
	}{
		{name: "saved", body: `{"status":"saved"}`, want: "saved", wantSts: http.StatusOK},
		{name: "applied", body: `{"status":"applied"}`, want: "applied", wantSts: http.StatusOK},
		{name: "empty", body: `{"status":""}`, want: "", wantSts: http.StatusOK},
		{name: "malformed JSON", body: `not-json`, wantSts: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRestSvc{}
			r := newRouter(fake)
			req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/status", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.wantSts {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantSts)
			}
			if tc.wantSts == http.StatusOK && fake.setStatusValue != tc.want {
				t.Errorf("status forwarded = %q, want %q", fake.setStatusValue, tc.want)
			}
		})
	}
}

func TestRest_SetNotes_DecodesBody(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantSts int
	}{
		{name: "non-empty notes", body: `{"notes":"hello"}`, want: "hello", wantSts: http.StatusOK},
		{name: "empty notes", body: `{"notes":""}`, want: "", wantSts: http.StatusOK},
		{name: "unicode notes", body: `{"notes":"日本語 — emoji 🚀"}`, want: "日本語 — emoji 🚀", wantSts: http.StatusOK},
		{name: "malformed JSON", body: `{`, wantSts: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRestSvc{}
			r := newRouter(fake)
			req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/notes", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.wantSts {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantSts)
			}
			if tc.wantSts == http.StatusOK && fake.setNotesValue != tc.want {
				t.Errorf("notes forwarded = %q, want %q", fake.setNotesValue, tc.want)
			}
		})
	}
}

func TestRest_Dismiss_DefaultReason(t *testing.T) {
	// DELETE without a body must default to "user_dismissed" — the most
	// charitable per-user dismissal reason. Empty reason is also defaulted.
	tests := []struct {
		name       string
		body       string
		wantReason string
	}{
		{name: "no body", body: "", wantReason: jobcommon.DismissalReasonUserDismissed},
		{name: "empty reason", body: `{"reason":""}`, wantReason: jobcommon.DismissalReasonUserDismissed},
		{name: "explicit reason", body: `{"reason":"user_not_relevant"}`, wantReason: "user_not_relevant"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRestSvc{}
			r := newRouter(fake)
			var req *http.Request
			if tc.body == "" {
				req = httptest.NewRequest(http.MethodDelete, "/jobs/j1", nil)
			} else {
				req = httptest.NewRequest(http.MethodDelete, "/jobs/j1", strings.NewReader(tc.body))
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
			}
			if fake.dismissReason != tc.wantReason {
				t.Errorf("reason forwarded = %q, want %q", fake.dismissReason, tc.wantReason)
			}
			if fake.dismissID != "j1" {
				t.Errorf("dismiss id = %q, want j1", fake.dismissID)
			}
		})
	}
}

func TestRest_BulkDismiss_ForwardsNote(t *testing.T) {
	fake := &fakeRestSvc{}
	r := newRouter(fake)
	body, _ := json.Marshal(map[string]any{"ids": []string{"j1"}, "note": "user said skip"})
	req := httptest.NewRequest(http.MethodPost, "/jobs/dismiss/bulk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if fake.bulkNote != "user said skip" {
		t.Errorf("note = %q, want forwarded", fake.bulkNote)
	}
}

// -------- Error mapping --------

func TestRest_List_ValidationErrorMapsTo400(t *testing.T) {
	// ValidationErrors from downstream surface as a 400 with field-level detail.
	verr := apperr.ValidationErrors{{Field: "status", Code: "INVALID_VALUE", Message: "bad"}}
	r := newRouter(&fakeRestSvc{listErr: verr})
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestRest_Get_ServiceErrorPreservesCode(t *testing.T) {
	r := newRouter(&fakeRestSvc{getErr: apperr.ErrBadRequest.WithMessage("nope")})
	req := httptest.NewRequest(http.MethodGet, "/jobs/anything", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (from ServiceError.Status)", w.Code)
	}
}

// -------- /jobs/companies and /jobs/counts --------

// fakeRestSvcReturning is a tiny extension to fakeRestSvc that overrides
// counts + companies returns. Used to exercise the success paths of the
// /jobs/companies and /jobs/counts handlers.
type fakeRestSvcReturning struct {
	fakeRestSvc
	countsRes  jobdto.Counts
	countsErr  error
	companies  []string
	companyErr error
	getCounts  int
	getCompany int
}

func (f *fakeRestSvcReturning) GetCounts(_ context.Context) (jobdto.Counts, error) {
	f.getCounts++
	return f.countsRes, f.countsErr
}
func (f *fakeRestSvcReturning) GetCompanies(_ context.Context) ([]string, error) {
	f.getCompany++
	return f.companies, f.companyErr
}

func TestRest_Companies_HappyPath(t *testing.T) {
	fake := &fakeRestSvcReturning{companies: []string{"Acme", "Globex"}}
	r := newRouter(fake)
	req := httptest.NewRequest(http.MethodGet, "/jobs/companies", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if fake.getCompany != 1 {
		t.Errorf("GetCompanies calls = %d, want 1", fake.getCompany)
	}
	if !strings.Contains(w.Body.String(), "Acme") || !strings.Contains(w.Body.String(), "Globex") {
		t.Errorf("body missing companies: %s", w.Body.String())
	}
}

func TestRest_Companies_ServiceErrorMapsTo500(t *testing.T) {
	fake := &fakeRestSvcReturning{companyErr: apperr.ErrInternal.WithMessage("db down")}
	r := newRouter(fake)
	req := httptest.NewRequest(http.MethodGet, "/jobs/companies", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestRest_Counts_HappyPath(t *testing.T) {
	fake := &fakeRestSvcReturning{countsRes: jobdto.Counts{Saved: 3, Applied: 1}}
	r := newRouter(fake)
	req := httptest.NewRequest(http.MethodGet, "/jobs/counts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if fake.getCounts != 1 {
		t.Errorf("GetCounts calls = %d, want 1", fake.getCounts)
	}
	if !strings.Contains(w.Body.String(), `"saved":3`) {
		t.Errorf("body missing saved count: %s", w.Body.String())
	}
}

func TestRest_Counts_ServiceErrorMapsTo500(t *testing.T) {
	fake := &fakeRestSvcReturning{countsErr: apperr.ErrInternal.WithMessage("counts down")}
	r := newRouter(fake)
	req := httptest.NewRequest(http.MethodGet, "/jobs/counts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// TestRest_Get_ServiceErrorMapsTo500 covers the get handler's HandleError
// branch with an INTERNAL error (the existing test only covers NOT_FOUND).
func TestRest_Get_ServiceErrorMapsTo500(t *testing.T) {
	r := newRouter(&fakeRestSvc{getErr: apperr.ErrInternal.WithMessage("db")})
	req := httptest.NewRequest(http.MethodGet, "/jobs/any", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// TestRest_SetSaved_ServiceErrorMapsToStatus — error returned from the
// service is passed to HandleError, mapping the ServiceError.Status correctly.
func TestRest_SetSaved_ServiceErrorMapsToStatus(t *testing.T) {
	fake := &fakeRestSvc{setSavedErr: apperr.ErrInternal.WithMessage("boom")}
	r := newRouter(fake)
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/save", strings.NewReader(`{"saved":true}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// TestRest_SetStatus_ServiceErrorMapsToStatus
func TestRest_SetStatus_ServiceErrorMapsToStatus(t *testing.T) {
	fake := &fakeRestSvc{setStatusErr: apperr.ErrBadRequest.WithMessage("bad")}
	r := newRouter(fake)
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/status", strings.NewReader(`{"status":"saved"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// TestRest_SetNotes_ServiceErrorMapsToStatus
func TestRest_SetNotes_ServiceErrorMapsToStatus(t *testing.T) {
	fake := &fakeRestSvc{setNotesErr: apperr.ErrInternal.WithMessage("db")}
	r := newRouter(fake)
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/notes", strings.NewReader(`{"notes":"hi"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// TestRest_Dismiss_ServiceErrorMapsToStatus
func TestRest_Dismiss_ServiceErrorMapsToStatus(t *testing.T) {
	fake := &fakeRestSvc{dismissErr: apperr.ErrNotFound.WithMessage("missing")}
	r := newRouter(fake)
	req := httptest.NewRequest(http.MethodDelete, "/jobs/j1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// TestRest_BulkDismiss_ServiceErrorMapsToStatus
func TestRest_BulkDismiss_ServiceErrorMapsToStatus(t *testing.T) {
	fake := &fakeRestSvc{bulkErr: apperr.ErrInternal.WithMessage("boom")}
	r := newRouter(fake)
	req := httptest.NewRequest(http.MethodPost, "/jobs/dismiss/bulk", strings.NewReader(`{"ids":["j1"]}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
