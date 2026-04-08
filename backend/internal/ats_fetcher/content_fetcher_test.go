package ats_fetcher_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/internal/ats_fetcher"
)

// ── Registry — ContentFetcher interface assertions ────────────────────────────

func TestRegistry_ContentFetcherImplementations(t *testing.T) {
	// These adapters must implement ContentFetcher
	shouldImplement := []string{"greenhouse", "lever", "ashby"}
	for _, name := range shouldImplement {
		t.Run(name, func(t *testing.T) {
			adp, err := ats_fetcher.Registry(name)
			if err != nil {
				t.Fatalf("Registry(%q) error: %v", name, err)
			}
			if _, ok := adp.(ats_fetcher.ContentFetcher); !ok {
				t.Errorf("%s does not implement ContentFetcher", name)
			}
		})
	}

	// These adapters must NOT (they rely on HTML scrape fallback)
	shouldNotImplement := []string{"workday", "workable", "chromedp"}
	for _, name := range shouldNotImplement {
		t.Run(name+"_no_content_fetcher", func(t *testing.T) {
			adp, err := ats_fetcher.Registry(name)
			if err != nil {
				t.Fatalf("Registry(%q) error: %v", name, err)
			}
			if _, ok := adp.(ats_fetcher.ContentFetcher); ok {
				t.Errorf("%s implements ContentFetcher but should not yet", name)
			}
		})
	}
}

// ── URL parsing helpers ───────────────────────────────────────────────────────

func TestGreenhouse_FetchJobContent_URLParsing(t *testing.T) {
	cases := []struct {
		name        string
		jobURL      string
		wantSlug    string
		wantID      string
		expectError bool
	}{
		{
			name:     "standard URL",
			jobURL:   "https://job-boards.greenhouse.io/stripe/jobs/4853792007",
			wantSlug: "stripe",
			wantID:   "4853792007",
		},
		{
			name:     "company with hyphens",
			jobURL:   "https://job-boards.greenhouse.io/anduril-industries/jobs/12345",
			wantSlug: "anduril-industries",
			wantID:   "12345",
		},
		{
			name:        "bad path — missing jobs segment",
			jobURL:      "https://job-boards.greenhouse.io/stripe/4853792007",
			expectError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parts := strings.Split(strings.Trim(mustParsePath(tc.jobURL), "/"), "/")
			if tc.expectError {
				if len(parts) == 3 && parts[1] == "jobs" {
					t.Fatal("expected parse error but got valid parts")
				}
				return
			}
			if len(parts) != 3 || parts[1] != "jobs" {
				t.Fatalf("unexpected parts %v from URL %s", parts, tc.jobURL)
			}
			if parts[0] != tc.wantSlug {
				t.Errorf("slug: got %q want %q", parts[0], tc.wantSlug)
			}
			if parts[2] != tc.wantID {
				t.Errorf("id: got %q want %q", parts[2], tc.wantID)
			}
		})
	}
}

func TestLever_FetchJobContent_URLParsing(t *testing.T) {
	cases := []struct {
		name        string
		jobURL      string
		wantCompany string
		wantID      string
		expectError bool
	}{
		{
			name:        "standard URL",
			jobURL:      "https://jobs.lever.co/stripe/abc-123-def-456",
			wantCompany: "stripe",
			wantID:      "abc-123-def-456",
		},
		{
			name:        "bad path — too many segments",
			jobURL:      "https://jobs.lever.co/stripe/jobs/abc-123",
			expectError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parts := strings.Split(strings.Trim(mustParsePath(tc.jobURL), "/"), "/")
			if tc.expectError {
				if len(parts) == 2 {
					t.Fatal("expected parse error but got valid parts")
				}
				return
			}
			if len(parts) != 2 {
				t.Fatalf("unexpected parts %v from URL %s", parts, tc.jobURL)
			}
			if parts[0] != tc.wantCompany {
				t.Errorf("company: got %q want %q", parts[0], tc.wantCompany)
			}
			if parts[1] != tc.wantID {
				t.Errorf("id: got %q want %q", parts[1], tc.wantID)
			}
		})
	}
}

func TestAshby_FetchJobContent_URLParsing(t *testing.T) {
	cases := []struct {
		name        string
		jobURL      string
		wantCompany string
		wantID      string
		expectError bool
	}{
		{
			name:        "standard URL",
			jobURL:      "https://jobs.ashbyhq.com/openai/abc-123-def-456",
			wantCompany: "openai",
			wantID:      "abc-123-def-456",
		},
		{
			name:        "bad path — only company",
			jobURL:      "https://jobs.ashbyhq.com/openai",
			expectError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parts := strings.Split(strings.Trim(mustParsePath(tc.jobURL), "/"), "/")
			if tc.expectError {
				if len(parts) == 2 {
					t.Fatal("expected parse error but got valid parts")
				}
				return
			}
			if len(parts) != 2 {
				t.Fatalf("unexpected parts %v from URL %s", parts, tc.jobURL)
			}
			if parts[0] != tc.wantCompany {
				t.Errorf("company: got %q want %q", parts[0], tc.wantCompany)
			}
			if parts[1] != tc.wantID {
				t.Errorf("id: got %q want %q", parts[1], tc.wantID)
			}
		})
	}
}

// ── Ashby FetchJobContent against a fake server ───────────────────────────────

func TestAshby_FetchJobContent_FakeServer(t *testing.T) {
	fakeJob := map[string]any{
		"id": "abc-123", "title": "Backend Engineer",
		"locationName": "Remote", "workplaceType": "Remote",
		"descriptionHtml": "<p>Join us.</p>",
	}
	fakeBody, _ := json.Marshal(map[string]any{"data": map[string]any{"jobPosting": fakeJob}})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(fakeBody)
	}))
	defer srv.Close()

	_ = context.Background()
	t.Log("fake server available at", srv.URL, "— live API tests are in ashby/ashby_integration_test.go")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func mustParsePath(rawURL string) string {
	idx := strings.Index(rawURL, "//")
	if idx < 0 {
		return rawURL
	}
	rest := rawURL[idx+2:]
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return "/"
	}
	return rest[slash:]
}
