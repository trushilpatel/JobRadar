package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	ats "github.com/trushilpatel/jobradar/apps/crawler/ats"
	google "github.com/trushilpatel/jobradar/apps/crawler/ats/google"
	logger "github.com/trushilpatel/jobradar/common/logger"
)

// --- Mock adapter types ---

type mockAdapter struct {
	fetchFn func(ctx context.Context, companyID string) (ats.FetchJobsResponse, error)
}

func (m *mockAdapter) FetchJobs(ctx context.Context, companyID string) (ats.FetchJobsResponse, error) {
	return m.fetchFn(ctx, companyID)
}

type mockStreamingAdapter struct {
	mockAdapter
	streamFn func([]ats.FetchJobsItem)
}

func (m *mockStreamingAdapter) SetStreamPublish(fn func([]ats.FetchJobsItem)) {
	m.streamFn = fn
}

func (m *mockStreamingAdapter) FetchJobs(ctx context.Context, companyID string) (ats.FetchJobsResponse, error) {
	return m.fetchFn(ctx, companyID)
}

type mockKnownAdapter struct {
	mockAdapter
	injectedIDs map[string]struct{}
}

func (m *mockKnownAdapter) SetKnownJobIDs(ids map[string]struct{}) {
	m.injectedIDs = ids
}

type mockSeenCheckerAdapter struct {
	mockAdapter
	seenFn func(ctx context.Context, ids []string) (map[string]bool, error)
}

func (m *mockSeenCheckerAdapter) SetSeenChecker(fn func(ctx context.Context, ids []string) (map[string]bool, error)) {
	m.seenFn = fn
}

type mockConfigurableAdapter struct {
	mockAdapter
	configFetchFn func(ctx context.Context, companyID string, config json.RawMessage) (ats.FetchJobsResponse, error)
}

func (m *mockConfigurableAdapter) FetchJobsWithConfig(ctx context.Context, companyID string, config json.RawMessage) (ats.FetchJobsResponse, error) {
	return m.configFetchFn(ctx, companyID, config)
}

// --- Mock OrchestratorClient ---

type mockBatchClient struct {
	batches []PageBatch
	err     error
}

func (m *mockBatchClient) GetWork(ctx context.Context, holding []string) (*GetWorkResult, error) {
	return &GetWorkResult{}, nil
}

func (m *mockBatchClient) SubmitBatch(ctx context.Context, batch PageBatch) error {
	m.batches = append(m.batches, batch)
	return m.err
}

func (m *mockBatchClient) ReportError(ctx context.Context, e WorkError) error { return nil }

// --- Helper ---

func makeExecutor(t *testing.T, adp ats.Adapter, client OrchestratorClient) AdapterExecutor {
	t.Helper()
	lookup := func(_ string, _ google.Config, _ logger.Logger) (ats.Adapter, error) {
		return adp, nil
	}
	return newAdapterExecutor(client, lookup, google.Config{}, logger.NewNoop())
}

func makeExecutorWithLookup(
	t *testing.T,
	lookup func(string, google.Config, logger.Logger) (ats.Adapter, error),
	client OrchestratorClient,
) AdapterExecutor {
	t.Helper()
	return newAdapterExecutor(client, lookup, google.Config{}, logger.NewNoop())
}

func workUnit(adapterType string, knownIDs []string) WorkUnit {
	return WorkUnit{
		AssignmentID: "assign-1",
		CompanyID:    "company-1",
		AdapterType:  adapterType,
		KnownIDs:     knownIDs,
	}
}

// --- Tests ---

func TestAdapterExecutor_NonStreaming_SubmitsSingleFinalBatch(t *testing.T) {
	jobs := []ats.FetchJobsItem{
		{ID: "lever-abc", Title: "SWE", URL: "https://example.com"},
	}
	adp := &mockAdapter{fetchFn: func(_ context.Context, _ string) (ats.FetchJobsResponse, error) {
		return ats.FetchJobsResponse{Jobs: jobs}, nil
	}}
	client := &mockBatchClient{}
	exec := makeExecutor(t, adp, client)

	if err := exec.Execute(context.Background(), workUnit("lever", nil)); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if len(client.batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(client.batches))
	}
	b := client.batches[0]
	if !b.Final {
		t.Error("expected Final=true on non-streaming batch")
	}
	if len(b.Jobs) != 1 {
		t.Errorf("expected 1 job, got %d", len(b.Jobs))
	}
	if b.Jobs[0].ID != "lever-abc" {
		t.Errorf("job ID = %q, want lever-abc", b.Jobs[0].ID)
	}
	if b.AssignmentID != "assign-1" {
		t.Errorf("AssignmentID = %q, want assign-1", b.AssignmentID)
	}
}

func TestAdapterExecutor_NonStreaming_MapsJobFields(t *testing.T) {
	fc := &ats.FullJobContent{
		Description:  "desc",
		Requirements: "reqs",
		Compensation: "100k",
		Department:   "eng",
	}
	adp := &mockAdapter{fetchFn: func(_ context.Context, _ string) (ats.FetchJobsResponse, error) {
		return ats.FetchJobsResponse{Jobs: []ats.FetchJobsItem{
			{
				ID:             "lever-abc",
				Title:          "SWE",
				URL:            "https://example.com",
				Location:       "Remote",
				EmploymentType: "full_time",
				LocationKnown:  true,
				AllLocations:   []string{"US", "CA"},
				FullContent:    fc,
			},
		}}, nil
	}}
	client := &mockBatchClient{}
	exec := makeExecutor(t, adp, client)

	if err := exec.Execute(context.Background(), workUnit("lever", nil)); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	j := client.batches[0].Jobs[0]
	if j.Location != "Remote" {
		t.Errorf("Location = %q, want Remote", j.Location)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if !j.LocationKnown {
		t.Error("LocationKnown should be true")
	}
	if len(j.AllLocations) != 2 {
		t.Errorf("AllLocations = %v, want 2", j.AllLocations)
	}
	if j.FullContent == nil {
		t.Fatal("expected non-nil FullContent")
	}
	if j.FullContent.Description != "desc" {
		t.Errorf("Description = %q, want desc", j.FullContent.Description)
	}
	if j.FullContent.Salary != "100k" {
		t.Errorf("Salary = %q, want 100k", j.FullContent.Salary)
	}
	if j.FullContent.Department != "eng" {
		t.Errorf("Department = %q, want eng", j.FullContent.Department)
	}
}

func TestAdapterExecutor_NonStreaming_PreviousKnownIDsSet(t *testing.T) {
	adp := &mockAdapter{fetchFn: func(_ context.Context, _ string) (ats.FetchJobsResponse, error) {
		return ats.FetchJobsResponse{}, nil
	}}
	client := &mockBatchClient{}
	exec := makeExecutor(t, adp, client)

	unit := workUnit("lever", []string{"lever-old1", "lever-old2"})
	if err := exec.Execute(context.Background(), unit); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if len(client.batches[0].PreviousKnownIDs) != 2 {
		t.Errorf("PreviousKnownIDs = %v, want 2 IDs", client.batches[0].PreviousKnownIDs)
	}
}

func TestAdapterExecutor_Streaming_SendsIntermediateAndTerminalBatches(t *testing.T) {
	adp := &mockStreamingAdapter{
		mockAdapter: mockAdapter{
			fetchFn: func(_ context.Context, _ string) (ats.FetchJobsResponse, error) {
				return ats.FetchJobsResponse{}, nil
			},
		},
	}
	client := &mockBatchClient{}
	exec := makeExecutor(t, adp, client)

	// Simulate the adapter calling its stream publish callback during FetchJobs.
	page1 := []ats.FetchJobsItem{{ID: "lever-p1"}}
	page2 := []ats.FetchJobsItem{{ID: "lever-p2"}}
	adp.fetchFn = func(_ context.Context, _ string) (ats.FetchJobsResponse, error) {
		// Mimic adapter streaming two pages.
		adp.streamFn(page1)
		adp.streamFn(page2)
		return ats.FetchJobsResponse{}, nil // streamed adapters may return empty result
	}

	if err := exec.Execute(context.Background(), workUnit("meta", nil)); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	// 2 intermediate + 1 terminal
	if len(client.batches) != 3 {
		t.Fatalf("expected 3 batches (2 pages + terminal), got %d", len(client.batches))
	}
	if client.batches[0].Final {
		t.Error("first batch should NOT be final")
	}
	if client.batches[1].Final {
		t.Error("second batch should NOT be final")
	}
	if !client.batches[2].Final {
		t.Error("terminal batch should be final")
	}
	if len(client.batches[2].Jobs) != 0 {
		t.Errorf("terminal batch should have no jobs, got %d", len(client.batches[2].Jobs))
	}
}

func TestAdapterExecutor_Streaming_FetchedCountIsAccumulated(t *testing.T) {
	adp := &mockStreamingAdapter{
		mockAdapter: mockAdapter{},
	}
	client := &mockBatchClient{}
	exec := makeExecutor(t, adp, client)

	adp.fetchFn = func(_ context.Context, _ string) (ats.FetchJobsResponse, error) {
		adp.streamFn([]ats.FetchJobsItem{{ID: "j1"}, {ID: "j2"}})
		adp.streamFn([]ats.FetchJobsItem{{ID: "j3"}})
		return ats.FetchJobsResponse{}, nil
	}

	if err := exec.Execute(context.Background(), workUnit("google", nil)); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	terminal := client.batches[len(client.batches)-1]
	if terminal.FetchedCount != 3 {
		t.Errorf("FetchedCount in terminal = %d, want 3", terminal.FetchedCount)
	}
}

func TestAdapterExecutor_ConfigurableAdapter_UsesWithConfig(t *testing.T) {
	var receivedConfig json.RawMessage
	adp := &mockConfigurableAdapter{
		mockAdapter: mockAdapter{fetchFn: func(_ context.Context, _ string) (ats.FetchJobsResponse, error) {
			return ats.FetchJobsResponse{}, nil
		}},
		configFetchFn: func(_ context.Context, _ string, cfg json.RawMessage) (ats.FetchJobsResponse, error) {
			receivedConfig = cfg
			return ats.FetchJobsResponse{Jobs: []ats.FetchJobsItem{{ID: "wd-1"}}}, nil
		},
	}
	client := &mockBatchClient{}
	exec := makeExecutor(t, adp, client)

	cfg := `{"host":"acme.wd3.myworkdayjobs.com"}`
	unit := workUnit("workday", nil)
	unit.AdapterConfig = &cfg

	if err := exec.Execute(context.Background(), unit); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if string(receivedConfig) != cfg {
		t.Errorf("config = %q, want %q", receivedConfig, cfg)
	}
	if len(client.batches[0].Jobs) != 1 {
		t.Errorf("expected 1 job, got %d", len(client.batches[0].Jobs))
	}
}

func TestAdapterExecutor_KnownJobsProviderInjected(t *testing.T) {
	adp := &mockKnownAdapter{
		mockAdapter: mockAdapter{fetchFn: func(_ context.Context, _ string) (ats.FetchJobsResponse, error) {
			return ats.FetchJobsResponse{}, nil
		}},
	}
	client := &mockBatchClient{}
	exec := makeExecutor(t, adp, client)

	unit := workUnit("lever", []string{"lever-a", "lever-b"})
	if err := exec.Execute(context.Background(), unit); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if len(adp.injectedIDs) != 2 {
		t.Errorf("injected IDs = %d, want 2", len(adp.injectedIDs))
	}
	if _, ok := adp.injectedIDs["lever-a"]; !ok {
		t.Error("lever-a not in injected IDs")
	}
}

func TestAdapterExecutor_SeenCheckerUsesKnownIDsSnapshot(t *testing.T) {
	adp := &mockSeenCheckerAdapter{
		mockAdapter: mockAdapter{fetchFn: func(_ context.Context, _ string) (ats.FetchJobsResponse, error) {
			return ats.FetchJobsResponse{}, nil
		}},
	}
	client := &mockBatchClient{}
	exec := makeExecutor(t, adp, client)

	unit := workUnit("github", []string{"github-known"})
	if err := exec.Execute(context.Background(), unit); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if adp.seenFn == nil {
		t.Fatal("seenFn not set")
	}
	result, err := adp.seenFn(context.Background(), []string{"github-known", "github-new"})
	if err != nil {
		t.Fatalf("seenFn error: %v", err)
	}
	if !result["github-known"] {
		t.Error("github-known should be in seen set")
	}
	if result["github-new"] {
		t.Error("github-new should NOT be in seen set")
	}
}

func TestAdapterExecutor_UnknownAdapter_ReturnsError(t *testing.T) {
	lookup := func(adapterType string, _ google.Config, _ logger.Logger) (ats.Adapter, error) {
		return nil, errors.New("unknown adapter")
	}
	client := &mockBatchClient{}
	exec := makeExecutorWithLookup(t, lookup, client)

	if err := exec.Execute(context.Background(), workUnit("badtype", nil)); err == nil {
		t.Error("expected error for unknown adapter type, got nil")
	}
}

func TestAdapterExecutor_FetchError_ReturnsError(t *testing.T) {
	adp := &mockAdapter{fetchFn: func(_ context.Context, _ string) (ats.FetchJobsResponse, error) {
		return ats.FetchJobsResponse{}, errors.New("fetch failed")
	}}
	client := &mockBatchClient{}
	exec := makeExecutor(t, adp, client)

	if err := exec.Execute(context.Background(), workUnit("lever", nil)); err == nil {
		t.Error("expected error for fetch failure, got nil")
	}
}

func TestAdapterExecutor_SubmitBatchError_ReturnsError(t *testing.T) {
	adp := &mockAdapter{fetchFn: func(_ context.Context, _ string) (ats.FetchJobsResponse, error) {
		return ats.FetchJobsResponse{Jobs: []ats.FetchJobsItem{{ID: "j1"}}}, nil
	}}
	client := &mockBatchClient{err: errors.New("network error")}
	exec := makeExecutor(t, adp, client)

	if err := exec.Execute(context.Background(), workUnit("lever", nil)); err == nil {
		t.Error("expected error from SubmitBatch, got nil")
	}
}

func TestAdapterExecutor_NilFullContent_MappedToNil(t *testing.T) {
	adp := &mockAdapter{fetchFn: func(_ context.Context, _ string) (ats.FetchJobsResponse, error) {
		return ats.FetchJobsResponse{Jobs: []ats.FetchJobsItem{
			{ID: "j1", FullContent: nil},
		}}, nil
	}}
	client := &mockBatchClient{}
	exec := makeExecutor(t, adp, client)

	if err := exec.Execute(context.Background(), workUnit("lever", nil)); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if client.batches[0].Jobs[0].FullContent != nil {
		t.Error("FullContent should be nil when source is nil")
	}
}
