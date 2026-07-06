package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/genai"

	llmerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/llm/dto"
)

// newTestGeminiScorer builds a *geminiScorer pointed at the supplied test
// server. It mirrors newGeminiScorer but injects HTTPOptions.BaseURL so the
// SDK talks to httptest instead of the real Gemini endpoint.
func newTestGeminiScorer(t *testing.T, baseURL, model string, roles []string, years int) *geminiScorer {
	t.Helper()
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-api-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: baseURL},
	})
	if err != nil {
		t.Fatalf("create test client: %v", err)
	}
	return &geminiScorer{
		client:      client,
		modelName:   model,
		targetRoles: roles,
		yearsExp:    years,
	}
}

// mldevResponse builds the minimal JSON body the genai SDK accepts as a
// successful generateContent response. The model "text" goes into the first
// candidate's first part.
func mldevResponse(text string) string {
	body := map[string]any{
		"candidates": []any{
			map[string]any{
				"content": map[string]any{
					"parts": []any{
						map[string]any{"text": text},
					},
					"role": "model",
				},
				"finishReason": "STOP",
			},
		},
	}
	b, _ := json.Marshal(body)
	return string(b)
}

// ─── ScoreBatch ──────────────────────────────────────────────────────────────

func TestGemini_ScoreBatch_EmptyInputReturnsNil(t *testing.T) {
	// Skip the network entirely — empty input is handled before SDK call.
	s := newTestGeminiScorer(t, "http://127.0.0.1:0", "m1", []string{"SWE"}, 3)
	out, err := s.ScoreBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil output for empty input, got %v", out)
	}
}

func TestGemini_ScoreBatch_HappyPath(t *testing.T) {
	var seenPath atomic.Value
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath.Store(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mldevResponse(`["a","b"]`)))
	}))
	defer ts.Close()

	s := newTestGeminiScorer(t, ts.URL, "gemini-3.1-flash-lite-preview", []string{"SWE"}, 3)
	out, err := s.ScoreBatch(context.Background(), []dto.JobsRelevanceFilterInput{
		{ID: "a", Title: "Senior SWE", Location: "NYC"},
		{ID: "b", Title: "Backend Engineer", Location: ""},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 || out[0] != "a" || out[1] != "b" {
		t.Fatalf("unexpected output: %v", out)
	}
	if p, ok := seenPath.Load().(string); !ok || !strings.Contains(p, "gemini-3.1-flash-lite-preview:generateContent") {
		t.Fatalf("expected request path to contain model + :generateContent, got %v", p)
	}
}

func TestGemini_ScoreBatch_EmptyLocationDefaultsToUnknown(t *testing.T) {
	var capturedBody atomic.Value
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		capturedBody.Store(string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mldevResponse(`[]`)))
	}))
	defer ts.Close()

	s := newTestGeminiScorer(t, ts.URL, "m1", []string{"SWE"}, 3)
	_, err := s.ScoreBatch(context.Background(), []dto.JobsRelevanceFilterInput{
		{ID: "x", Title: "T", Location: ""},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, _ := capturedBody.Load().(string)
	if !strings.Contains(body, "Location: Unknown") {
		t.Fatalf("expected empty location to be normalised to 'Unknown' in prompt body, got %q", body)
	}
}

func TestGemini_ScoreBatch_RejectsUnparseableJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Bare text — fails to unmarshal into []string.
		_, _ = w.Write([]byte(mldevResponse(`not json at all`)))
	}))
	defer ts.Close()

	s := newTestGeminiScorer(t, ts.URL, "m1", []string{"SWE"}, 3)
	_, err := s.ScoreBatch(context.Background(), []dto.JobsRelevanceFilterInput{{ID: "j", Title: "T"}})
	if err == nil {
		t.Fatal("expected JSON parse error")
	}
	if !strings.Contains(err.Error(), "gemini score batch: parse response") {
		t.Fatalf("error should mention parse response, got %v", err)
	}
}

func TestGemini_ScoreBatch_EmptyResponseTextIsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Return candidates with no parts -> Text() returns "".
		body := `{"candidates":[{"content":{"parts":[],"role":"model"},"finishReason":"STOP"}]}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	s := newTestGeminiScorer(t, ts.URL, "m1", []string{"SWE"}, 3)
	_, err := s.ScoreBatch(context.Background(), []dto.JobsRelevanceFilterInput{{ID: "j", Title: "T"}})
	if err == nil {
		t.Fatal("expected error on empty model response")
	}
	if !strings.Contains(err.Error(), "empty response from model") {
		t.Fatalf("error should mention empty response, got %v", err)
	}
}

func TestGemini_ScoreBatch_Maps429ToSentinel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":429,"message":"rate limited","status":"RESOURCE_EXHAUSTED"}}`))
	}))
	defer ts.Close()

	s := newTestGeminiScorer(t, ts.URL, "m1", []string{"SWE"}, 3)
	_, err := s.ScoreBatch(context.Background(), []dto.JobsRelevanceFilterInput{{ID: "j", Title: "T"}})
	if !errors.Is(err, llmerr.ErrLLMProviderRateLimited) {
		t.Fatalf("expected ErrLLMProviderRateLimited, got %v", err)
	}
}

func TestGemini_ScoreBatch_Maps503ToSentinel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"code":503,"message":"overloaded","status":"UNAVAILABLE"}}`))
	}))
	defer ts.Close()

	s := newTestGeminiScorer(t, ts.URL, "m1", []string{"SWE"}, 3)
	_, err := s.ScoreBatch(context.Background(), []dto.JobsRelevanceFilterInput{{ID: "j", Title: "T"}})
	if !errors.Is(err, llmerr.ErrLLMProviderRateLimited) {
		t.Fatalf("expected 503 mapped to ErrLLMProviderRateLimited, got %v", err)
	}
}

func TestGemini_ScoreBatch_Bubbles500AsRawError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":500,"message":"boom","status":"INTERNAL"}}`))
	}))
	defer ts.Close()

	s := newTestGeminiScorer(t, ts.URL, "m1", []string{"SWE"}, 3)
	_, err := s.ScoreBatch(context.Background(), []dto.JobsRelevanceFilterInput{{ID: "j", Title: "T"}})
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, llmerr.ErrLLMProviderRateLimited) {
		t.Fatalf("500 must NOT map to ErrLLMProviderRateLimited; got %v", err)
	}
}

// ─── EnrichBatch ─────────────────────────────────────────────────────────────

func TestGemini_EnrichBatch_EmptyInputReturnsNil(t *testing.T) {
	s := newTestGeminiScorer(t, "http://127.0.0.1:0", "m1", []string{"SWE"}, 3)
	out, err := s.EnrichBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil output for empty input, got %v", out)
	}
}

func TestGemini_EnrichBatch_HappyPath(t *testing.T) {
	// Model returns a JSON array with one well-formed result per input.
	resp := `[
		{"job_id":"j1","experience_years":3,"tech_stack":["Go"],"salary_min":null,"salary_max":null,"salary_currency":"","summary":"s","remote":true,"country":"USA","state":null,"sponsorship":true,"requires_citizen":false,"security_clearance":false,"role_brief":"b","responsibilities":["r"],"role_category":"backend","role_match":true}
	]`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mldevResponse(resp)))
	}))
	defer ts.Close()

	s := newTestGeminiScorer(t, ts.URL, "gemma-4-31b-it", []string{"SWE"}, 3)
	out, err := s.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j1", RawText: "raw"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0] == nil || out[0].JobID != "j1" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if !out[0].Remote || out[0].RoleCategory != "backend" || !out[0].RoleMatch {
		t.Fatalf("unexpected enrichment fields: %+v", out[0])
	}
}

func TestGemini_EnrichBatch_StripsMarkdownFencesViaCleanJSON(t *testing.T) {
	// Model returns the array wrapped in ```json ... ``` fences and trailing
	// noise. cleanJSON trims down to the [ ... ] payload.
	innerArray := `[{"job_id":"x","experience_years":null,"tech_stack":[],"salary_min":null,"salary_max":null,"salary_currency":"","summary":"","remote":false,"country":"","state":null,"sponsorship":true,"requires_citizen":false,"security_clearance":false,"role_brief":"","responsibilities":[],"role_category":"other","role_match":false}]`
	resp := "```json\n" + innerArray + "\n```\nignored"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mldevResponse(resp)))
	}))
	defer ts.Close()

	s := newTestGeminiScorer(t, ts.URL, "m1", []string{"SWE"}, 3)
	out, err := s.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "x", RawText: "raw"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].JobID != "x" {
		t.Fatalf("expected single result with job_id=x, got %+v", out)
	}
}

func TestGemini_EnrichBatch_Maps429ToSentinel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":429,"message":"rl","status":"RESOURCE_EXHAUSTED"}}`))
	}))
	defer ts.Close()

	s := newTestGeminiScorer(t, ts.URL, "m1", []string{"SWE"}, 3)
	_, err := s.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "x"}})
	if !errors.Is(err, llmerr.ErrLLMProviderRateLimited) {
		t.Fatalf("expected ErrLLMProviderRateLimited, got %v", err)
	}
}

func TestGemini_EnrichBatch_RejectsUnparseableJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mldevResponse(`not json`)))
	}))
	defer ts.Close()

	s := newTestGeminiScorer(t, ts.URL, "m1", []string{"SWE"}, 3)
	_, err := s.EnrichBatch(context.Background(), []dto.JobPostEnrichInput{{ID: "j", RawText: "raw"}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gemini enrich batch: parse response") {
		t.Fatalf("error should mention parse response, got %v", err)
	}
}

// ─── cleanJSON ───────────────────────────────────────────────────────────────

func TestCleanJSON_Variants(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain object", `{"a":1}`, `{"a":1}`},
		{"plain array", `[1,2]`, `[1,2]`},
		{"leading/trailing whitespace", "   \n[1,2]\t  ", `[1,2]`},
		{"non-breaking-space replaced with space", " [ 1 ]", `[ 1 ]`},
		{"markdown fence stripped to outer braces", "```json\n{\"x\":true}\n```", `{"x":true}`},
		{"trailing prose stripped after last bracket", "[1,2]ignored after", `[1,2]`},
		{"leading prose stripped before first brace", "prefix junk {\"a\":1}", `{"a":1}`},
		{"no brackets at all returns original", "no brackets here", `no brackets here`},
		{"start after end fallback returns original", "}{", "}{"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := cleanJSON(tc.in)
			if got != tc.want {
				t.Fatalf("cleanJSON(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ─── newGeminiScorer constructor ─────────────────────────────────────────────

func TestNewGeminiScorer_RejectsEmptyAPIKey(t *testing.T) {
	// Ensure the SDK does not fall back to a real env-provided key in tests.
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	_, err := newGeminiScorer(context.Background(), "", "m1", []string{"SWE"}, 3)
	if err == nil {
		t.Fatal("expected newGeminiScorer to reject empty API key")
	}
	if !strings.Contains(err.Error(), "gemini: create client") {
		t.Fatalf("expected wrapped 'gemini: create client' error, got %v", err)
	}
}

func TestNewGeminiScorer_AcceptsValidConfig(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	s, err := newGeminiScorer(context.Background(), "key", "model", []string{"r"}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil scorer")
	}
}
