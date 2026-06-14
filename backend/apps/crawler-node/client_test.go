package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/ats/core"
)

// orchestratorWorkResponse mirrors the orchestrator's dto.WorkResponse JSON shape.
type orchestratorWorkResponse struct {
	Assignment      *WorkUnit `json:"assignment"`
	HeartbeatAcked  []string  `json:"heartbeat_acked"`
	HeartbeatMissed []string  `json:"heartbeat_missed"`
}

func makeTestClient(t *testing.T, srv *httptest.Server) OrchestratorClient {
	t.Helper()
	cfg := &Config{
		OrchestratorURL:     srv.URL,
		CrawlerToken:        "test-token",
		HTTPTimeoutSeconds:  5,
		RetryMaxAttempts:    1, // no retries in most tests
		PollIntervalSeconds: 1,
	}
	return newHTTPClient(cfg)
}

// --- GetWork ---

func TestHTTPClient_GetWork_EmptyQueue_ReturnsNilAssignment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/node/work" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", auth)
		}
		resp := orchestratorWorkResponse{Assignment: nil, HeartbeatAcked: nil, HeartbeatMissed: nil}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": resp}) //nolint:errcheck
	}))
	defer srv.Close()

	client := makeTestClient(t, srv)
	result, err := client.GetWork(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetWork error: %v", err)
	}
	if result.Assignment != nil {
		t.Errorf("expected nil assignment, got %v", result.Assignment)
	}
}

func TestHTTPClient_GetWork_WithAssignment_ReturnsUnit(t *testing.T) {
	unit := &WorkUnit{
		AssignmentID: "assign-1",
		CompanyID:    "company-1",
		AdapterType:  "lever",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := orchestratorWorkResponse{Assignment: unit}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": resp}) //nolint:errcheck
	}))
	defer srv.Close()

	client := makeTestClient(t, srv)
	result, err := client.GetWork(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetWork error: %v", err)
	}
	if result.Assignment == nil {
		t.Fatal("expected non-nil assignment")
	}
	if result.Assignment.AssignmentID != "assign-1" {
		t.Errorf("AssignmentID = %q, want assign-1", result.Assignment.AssignmentID)
	}
}

func TestHTTPClient_GetWork_HoldingPassedAsQueryParams(t *testing.T) {
	var receivedHolding []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHolding = r.URL.Query()["holding"]
		resp := orchestratorWorkResponse{}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": resp}) //nolint:errcheck
	}))
	defer srv.Close()

	client := makeTestClient(t, srv)
	_, err := client.GetWork(context.Background(), []string{"id-1", "id-2"})
	if err != nil {
		t.Fatalf("GetWork error: %v", err)
	}
	if len(receivedHolding) != 2 {
		t.Errorf("holding params = %v, want [id-1 id-2]", receivedHolding)
	}
}

func TestHTTPClient_GetWork_HeartbeatMissedPropagated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := orchestratorWorkResponse{
			HeartbeatMissed: []string{"gone-1", "gone-2"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": resp}) //nolint:errcheck
	}))
	defer srv.Close()

	client := makeTestClient(t, srv)
	result, err := client.GetWork(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetWork error: %v", err)
	}
	if len(result.HeartbeatMissed) != 2 {
		t.Errorf("HeartbeatMissed = %v, want [gone-1 gone-2]", result.HeartbeatMissed)
	}
}

func TestHTTPClient_GetWork_ServerError_Retries(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		resp := orchestratorWorkResponse{}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": resp}) //nolint:errcheck
	}))
	defer srv.Close()

	cfg := &Config{
		OrchestratorURL:    srv.URL,
		CrawlerToken:       "tok",
		HTTPTimeoutSeconds: 5,
		RetryMaxAttempts:   3,
	}
	client := newHTTPClient(cfg)
	_, err := client.GetWork(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetWork error after retries: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls (2 failures + 1 success), got %d", calls)
	}
}

func TestHTTPClient_GetWork_ExhaustedRetries_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cfg := &Config{
		OrchestratorURL:    srv.URL,
		CrawlerToken:       "tok",
		HTTPTimeoutSeconds: 5,
		RetryMaxAttempts:   2,
	}
	client := newHTTPClient(cfg)
	_, err := client.GetWork(context.Background(), nil)
	if err == nil {
		t.Error("expected error after exhausted retries, got nil")
	}
}

// --- SubmitBatch ---

func TestHTTPClient_SubmitBatch_SendsJSON(t *testing.T) {
	var received PageBatch
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/node/results" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"published": 1}}) //nolint:errcheck
	}))
	defer srv.Close()

	client := makeTestClient(t, srv)
	batch := PageBatch{
		AssignmentID: "assign-1",
		CompanyID:    "company-1",
		AdapterType:  "lever",
		Jobs:         []core.JobSummary{{ID: "lever-abc", Title: "SWE"}},
		Final:        true,
	}
	if err := client.SubmitBatch(context.Background(), batch); err != nil {
		t.Fatalf("SubmitBatch error: %v", err)
	}
	if received.AssignmentID != "assign-1" {
		t.Errorf("AssignmentID = %q, want assign-1", received.AssignmentID)
	}
	if !received.Final {
		t.Error("Final should be true")
	}
}

func TestHTTPClient_SubmitBatch_ServerError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := makeTestClient(t, srv)
	if err := client.SubmitBatch(context.Background(), PageBatch{}); err == nil {
		t.Error("expected error for server 500, got nil")
	}
}

// --- ReportError ---

func TestHTTPClient_ReportError_SendsJSON(t *testing.T) {
	var received WorkError
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/node/error" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := makeTestClient(t, srv)
	werr := WorkError{
		AssignmentID: "assign-1",
		CompanyID:    "company-1",
		AdapterType:  "lever",
		ErrorMessage: "adapter timeout",
		DurationMs:   5000,
	}
	if err := client.ReportError(context.Background(), werr); err != nil {
		t.Fatalf("ReportError error: %v", err)
	}
	if received.ErrorMessage != "adapter timeout" {
		t.Errorf("ErrorMessage = %q, want 'adapter timeout'", received.ErrorMessage)
	}
}

func TestHTTPClient_ReportError_ServerError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := makeTestClient(t, srv)
	if err := client.ReportError(context.Background(), WorkError{}); err == nil {
		t.Error("expected error for server 500, got nil")
	}
}

// --- 4xx responses ---

func TestHTTPClient_GetWork_4xxError_ReturnsNonRetriableError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := makeTestClient(t, srv)
	_, err := client.GetWork(context.Background(), nil)
	if err == nil {
		t.Error("expected error for 4xx response, got nil")
	}
}

func TestHTTPClient_SubmitBatch_4xxError_ReturnsNonRetriableError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	client := makeTestClient(t, srv)
	if err := client.SubmitBatch(context.Background(), PageBatch{}); err == nil {
		t.Error("expected error for 4xx response, got nil")
	}
}

func TestHTTPClient_ReportError_4xxError_ReturnsNonRetriableError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := makeTestClient(t, srv)
	if err := client.ReportError(context.Background(), WorkError{}); err == nil {
		t.Error("expected error for 4xx response, got nil")
	}
}

// --- Malformed JSON response ---

func TestHTTPClient_GetWork_MalformedJSON_ReturnsDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not valid json`)) //nolint:errcheck
	}))
	defer srv.Close()

	client := makeTestClient(t, srv)
	_, err := client.GetWork(context.Background(), nil)
	if err == nil {
		t.Error("expected decode error for malformed JSON, got nil")
	}
}

// --- Invalid base URL ---

func TestHTTPClient_GetWork_InvalidBaseURL_ReturnsError(t *testing.T) {
	cfg := &Config{
		OrchestratorURL:    "://not-a-url",
		CrawlerToken:       "tok",
		HTTPTimeoutSeconds: 5,
		RetryMaxAttempts:   1,
	}
	client := newHTTPClient(cfg)
	_, err := client.GetWork(context.Background(), nil)
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}

// --- retriableError.Error() ---

func TestHTTPClient_ExhaustedRetries_ErrorContainsStatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable) // 503
	}))
	defer srv.Close()

	cfg := &Config{
		OrchestratorURL:    srv.URL,
		CrawlerToken:       "tok",
		HTTPTimeoutSeconds: 5,
		RetryMaxAttempts:   1,
	}
	client := newHTTPClient(cfg)
	_, err := client.GetWork(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	msg := err.Error()
	if msg == "" {
		t.Error("error message should not be empty")
	}
}

// --- Context cancellation ---

func TestHTTPClient_GetWork_ContextCancelled_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	client := makeTestClient(t, srv)
	_, err := client.GetWork(ctx, nil)
	if err == nil {
		t.Error("expected error on context cancellation, got nil")
	}
}
