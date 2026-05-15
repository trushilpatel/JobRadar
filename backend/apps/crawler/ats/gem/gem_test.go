package gem

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

// ── Interface compliance ────────────────────────────────────────────────────

func TestGem_ImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*Gem)(nil)
}

func TestGem_ImplementsURLProvider(t *testing.T) {
	var _ types.URLProvider = (*Gem)(nil)
}

func TestGem_DoesNotImplementContentFetcher(t *testing.T) {
	// Full-detail adapter: FullContent is populated at scan time, so a separate
	// ContentFetcher must NOT be implemented. The analyzer's type assertion
	// would otherwise trigger an unnecessary detail fetch.
	var v interface{} = New()
	if _, ok := v.(types.ContentFetcher); ok {
		t.Fatal("Gem must not implement ContentFetcher — it is a full-detail adapter")
	}
}

// ── Constructor ─────────────────────────────────────────────────────────────

func TestNew_DefaultBaseURL(t *testing.T) {
	g := New()
	if g.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q, want %q", g.baseURL, defaultBaseURL)
	}
}

func TestCareersURL(t *testing.T) {
	g := New()
	got := g.CareersURL("inception")
	want := "https://jobs.gem.com/inception"
	if got != want {
		t.Errorf("CareersURL(%q) = %q, want %q", "inception", got, want)
	}
}

func TestResolveBaseURL_ZeroValueFallsBackToDefault(t *testing.T) {
	g := &Gem{}
	if got := g.resolveBaseURL(); got != defaultBaseURL {
		t.Errorf("zero-value resolveBaseURL: got %q, want %q", got, defaultBaseURL)
	}
}

// ── FetchJobs — happy path ───────────────────────────────────────────────────

func TestFetchJobs_HappyPath(t *testing.T) {
	posts := []gemJobPost{
		{
			ID:               "am9icG9zdDqJOzJ0IJMTm7wfiKynjWyB",
			Title:            "Head of Product Management",
			AbsoluteURL:      "https://jobs.gem.com/inception/am9icG9zdDqJOzJ0IJMTm7wfiKynjWyB",
			Content:          "<div>Lead product</div>",
			ContentPlain:     "Lead product",
			Location:         gemLocation{Name: "San Mateo, United States"},
			LocationType:     "in_office",
			EmploymentType:   "full_time",
			Departments:      []gemTaxonomy{{ID: "d1", Name: "Product"}},
			Offices:          []gemOffice{{ID: "o1", Name: "Bay Area", Location: gemLocation{Name: "San Mateo, United States"}}},
			RequisitionID:    "R18",
			CreatedAt:        "2025-09-27T05:18:33.220Z",
			UpdatedAt:        "2026-04-28T21:47:09.672Z",
			FirstPublishedAt: "2025-09-27T16:26:37.086Z",
		},
		{
			ID:               "am9icG9zdDoTQImJJDO8v2f4YCHjQSUg",
			Title:            "ML Researcher",
			AbsoluteURL:      "https://jobs.gem.com/inception/am9icG9zdDoTQImJJDO8v2f4YCHjQSUg",
			Content:          "<p>Train models</p>",
			ContentPlain:     "Train models",
			Location:         gemLocation{Name: "Remote"},
			LocationType:     "remote",
			EmploymentType:   "intern",
			Departments:      []gemTaxonomy{{ID: "d2", Name: "Research"}},
			FirstPublishedAt: "2026-02-01T00:00:00.000Z",
		},
	}

	srv := newTestServer(t, map[string][]gemJobPost{"inception": posts})
	defer srv.Close()

	g := &Gem{baseURL: srv.URL}
	resp, err := g.FetchJobs(context.Background(), "inception")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(resp.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(resp.Jobs))
	}

	j0 := resp.Jobs[0]
	assertEqual(t, "ID", j0.ID, fmt.Sprintf("%s-%s", companycommon.AdapterGem, posts[0].ID))
	assertEqual(t, "Title", j0.Title, "Head of Product Management")
	assertEqual(t, "URL", j0.URL, posts[0].AbsoluteURL)
	assertEqual(t, "Location", j0.Location, "San Mateo, United States")
	assertEqual(t, "EmploymentType", j0.EmploymentType, jobcommon.EmployFullTime)
	if j0.FullContent == nil {
		t.Fatal("FullContent must not be nil — gem is a full-detail adapter")
	}
	assertEqual(t, "Description", j0.FullContent.Description, "Lead product")
	assertEqual(t, "Department", j0.FullContent.Department, "Product")
	assertEqual(t, "WorkplaceType", j0.FullContent.WorkplaceType, "onsite")
	assertEqual(t, "PublishedDate", j0.FullContent.PublishedDate, "2025-09-27T16:26:37.086Z")

	j1 := resp.Jobs[1]
	assertEqual(t, "EmploymentType intern", j1.EmploymentType, jobcommon.EmployInternship)
	assertEqual(t, "WorkplaceType remote", j1.FullContent.WorkplaceType, "remote")
}

// ── FetchJobs — empty array ─────────────────────────────────────────────────

func TestFetchJobs_EmptyArray(t *testing.T) {
	srv := newTestServer(t, map[string][]gemJobPost{"inception": {}})
	defer srv.Close()

	g := &Gem{baseURL: srv.URL}
	resp, err := g.FetchJobs(context.Background(), "inception")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(resp.Jobs))
	}
}

// ── FetchJobs — error paths ─────────────────────────────────────────────────

func TestFetchJobs_EmptySlug(t *testing.T) {
	g := New()
	if _, err := g.FetchJobs(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty slug")
	}
	if _, err := g.FetchJobs(context.Background(), "   "); err == nil {
		t.Fatal("expected error for whitespace-only slug")
	}
}

func TestFetchJobs_404UnknownSlug(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"not found"}`))
	}))
	defer srv.Close()

	g := &Gem{baseURL: srv.URL}
	_, err := g.FetchJobs(context.Background(), "no-such-board")
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404, got: %v", err)
	}
}

func TestFetchJobs_500ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	g := &Gem{baseURL: srv.URL}
	_, err := g.FetchJobs(context.Background(), "inception")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got %v", err)
	}
}

func TestFetchJobs_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	g := &Gem{baseURL: srv.URL}
	_, err := g.FetchJobs(context.Background(), "inception")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestFetchJobs_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	g := &Gem{baseURL: srv.URL}
	if _, err := g.FetchJobs(ctx, "inception"); err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// ── FetchJobs — URL construction & headers ──────────────────────────────────

func TestFetchJobs_RequestPathAndHeaders(t *testing.T) {
	var capturedPath, capturedAccept, capturedMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAccept = r.Header.Get("Accept")
		capturedMethod = r.Method
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	g := &Gem{baseURL: srv.URL}
	_, err := g.FetchJobs(context.Background(), "lumalabs-ai")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if capturedMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", capturedMethod)
	}
	if capturedPath != "/job_board/v0/lumalabs-ai/job_posts/" {
		t.Errorf("path = %q, want /job_board/v0/lumalabs-ai/job_posts/", capturedPath)
	}
	if capturedAccept != "application/json" {
		t.Errorf("Accept = %q, want application/json", capturedAccept)
	}
}

// ── resolveLocation ─────────────────────────────────────────────────────────

func TestResolveLocation(t *testing.T) {
	tests := []struct {
		name string
		in   gemJobPost
		want string
	}{
		{
			name: "primary location present",
			in:   gemJobPost{Location: gemLocation{Name: "New York, United States"}},
			want: "New York, United States",
		},
		{
			name: "primary empty, falls back to offices",
			in: gemJobPost{Offices: []gemOffice{
				{Location: gemLocation{Name: "Bay Area"}},
				{Location: gemLocation{Name: "London"}},
			}},
			want: "Bay Area; London",
		},
		{
			name: "primary empty, single office",
			in: gemJobPost{Offices: []gemOffice{
				{Location: gemLocation{Name: "Remote"}},
			}},
			want: "Remote",
		},
		{
			name: "primary empty, offices with blank names skipped",
			in: gemJobPost{Offices: []gemOffice{
				{Location: gemLocation{Name: ""}},
				{Location: gemLocation{Name: "Berlin"}},
			}},
			want: "Berlin",
		},
		{
			name: "everything empty",
			in:   gemJobPost{},
			want: "",
		},
		{
			name: "primary whitespace-only falls back to offices",
			in: gemJobPost{
				Location: gemLocation{Name: "   "},
				Offices:  []gemOffice{{Location: gemLocation{Name: "Tokyo"}}},
			},
			want: "Tokyo",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveLocation(tc.in); got != tc.want {
				t.Errorf("resolveLocation() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ── resolveDescription ──────────────────────────────────────────────────────

func TestResolveDescription(t *testing.T) {
	tests := []struct {
		name string
		in   gemJobPost
		want string
	}{
		{
			name: "content_plain preferred",
			in:   gemJobPost{Content: "<p>html</p>", ContentPlain: "plain"},
			want: "plain",
		},
		{
			name: "falls back to StripHTML when content_plain blank",
			in:   gemJobPost{Content: "<p>Hello&nbsp;<b>world</b></p>"},
			want: "Hello world",
		},
		{
			name: "whitespace-only content_plain falls back",
			in:   gemJobPost{Content: "<p>fallback</p>", ContentPlain: "   "},
			want: "fallback",
		},
		{
			name: "both empty",
			in:   gemJobPost{},
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveDescription(tc.in); got != tc.want {
				t.Errorf("resolveDescription() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ── resolvePublishedDate ────────────────────────────────────────────────────

func TestResolvePublishedDate(t *testing.T) {
	tests := []struct {
		name string
		in   gemJobPost
		want string
	}{
		{"first_published_at preferred", gemJobPost{FirstPublishedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-02-01T00:00:00Z", CreatedAt: "2025-12-01T00:00:00Z"}, "2026-01-01T00:00:00Z"},
		{"falls back to updated_at", gemJobPost{UpdatedAt: "2026-02-01T00:00:00Z", CreatedAt: "2025-12-01T00:00:00Z"}, "2026-02-01T00:00:00Z"},
		{"falls back to created_at", gemJobPost{CreatedAt: "2025-12-01T00:00:00Z"}, "2025-12-01T00:00:00Z"},
		{"all empty", gemJobPost{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolvePublishedDate(tc.in); got != tc.want {
				t.Errorf("resolvePublishedDate() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ── firstDepartment ─────────────────────────────────────────────────────────

func TestFirstDepartment(t *testing.T) {
	if got := firstDepartment(nil); got != "" {
		t.Errorf("nil -> %q, want empty", got)
	}
	if got := firstDepartment([]gemTaxonomy{}); got != "" {
		t.Errorf("empty -> %q, want empty", got)
	}
	if got := firstDepartment([]gemTaxonomy{{Name: "Engineering"}, {Name: "Research"}}); got != "Engineering" {
		t.Errorf("first dept = %q, want Engineering", got)
	}
}

// ── mapEmploymentType ───────────────────────────────────────────────────────

func TestMapEmploymentType(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"full_time", jobcommon.EmployFullTime},
		{"FULL_TIME", jobcommon.EmployFullTime},
		{" full_time ", jobcommon.EmployFullTime},
		{"part_time", jobcommon.EmployPartTime},
		{"contract", jobcommon.EmployContract},
		{"intern", jobcommon.EmployInternship},
		{"internship", jobcommon.EmployInternship},
		{"", jobcommon.EmployFullTime},
		{"freelance", jobcommon.EmployFullTime}, // unknown defaults to full_time
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := mapEmploymentType(tc.in); got != tc.want {
				t.Errorf("mapEmploymentType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ── mapWorkplaceType ────────────────────────────────────────────────────────

func TestMapWorkplaceType(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"remote", "remote"},
		{"REMOTE", "remote"},
		{"hybrid", "hybrid"},
		{"in_office", "onsite"},
		{"onsite", "onsite"},
		{"on_site", "onsite"},
		{"", ""},
		{"unknown", ""},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := mapWorkplaceType(tc.in); got != tc.want {
				t.Errorf("mapWorkplaceType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ── jobToItem — full mapping ────────────────────────────────────────────────

func TestJobToItem_FullMapping(t *testing.T) {
	in := gemJobPost{
		ID:               "abc",
		Title:            "T",
		AbsoluteURL:      "https://jobs.gem.com/x/abc",
		Content:          "<p>HTML</p>",
		ContentPlain:     "plain",
		Location:         gemLocation{Name: "SF"},
		LocationType:     "hybrid",
		EmploymentType:   "part_time",
		Departments:      []gemTaxonomy{{Name: "Eng"}},
		FirstPublishedAt: "2026-03-01T00:00:00Z",
	}
	got := jobToItem(in)
	want := types.FetchJobsItem{
		ID:             "gem-abc",
		Title:          "T",
		URL:            "https://jobs.gem.com/x/abc",
		Location:       "SF",
		EmploymentType: jobcommon.EmployPartTime,
		FullContent: &types.FullJobContent{
			Description:   "plain",
			Department:    "Eng",
			WorkplaceType: "hybrid",
			PublishedDate: "2026-03-01T00:00:00Z",
		},
	}
	if got.ID != want.ID || got.Title != want.Title || got.URL != want.URL ||
		got.Location != want.Location || got.EmploymentType != want.EmploymentType {
		t.Errorf("jobToItem mismatch:\n got=%+v\nwant=%+v", got, want)
	}
	if got.FullContent == nil || *got.FullContent != *want.FullContent {
		t.Errorf("FullContent mismatch:\n got=%+v\nwant=%+v", got.FullContent, want.FullContent)
	}
}

// ── Real integration test ───────────────────────────────────────────────────
// Guard: set TEST_REAL=1. Hits the live gem.com Job Board API.

func TestFetchJobs_Real(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping real API test; set TEST_REAL=1 to run")
	}

	g := New()
	resp, err := g.FetchJobs(context.Background(), "inception")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(resp.Jobs) == 0 {
		t.Fatal("expected at least one job from live gem.com API for 'inception'")
	}

	j := resp.Jobs[0]
	if !strings.HasPrefix(j.ID, "gem-") {
		t.Errorf("ID %q should have 'gem-' prefix", j.ID)
	}
	if j.Title == "" {
		t.Error("Title should not be empty")
	}
	if !strings.HasPrefix(j.URL, "https://jobs.gem.com/inception/") {
		t.Errorf("URL %q should start with https://jobs.gem.com/inception/", j.URL)
	}
	if j.FullContent == nil || j.FullContent.Description == "" {
		t.Error("FullContent.Description should not be empty (gem returns content at scan time)")
	}
	t.Logf("fetched %d jobs; first: %q (%s) at %q", len(resp.Jobs), j.Title, j.EmploymentType, j.Location)
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// newTestServer serves GET /job_board/v0/{slug}/job_posts/ returning the posts
// configured for that slug, or 404 for unknown slugs.
func newTestServer(t *testing.T, postsBySlug map[string][]gemJobPost) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		// Expected path: /job_board/v0/{slug}/job_posts/
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 4 || parts[0] != "job_board" || parts[1] != "v0" || parts[3] != "job_posts" {
			http.NotFound(w, r)
			return
		}
		slug := parts[2]
		posts, ok := postsBySlug[slug]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(posts)
	}))
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}
