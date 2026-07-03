package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters"
	"github.com/trushilpatel/jobradar/common/ats/core"
	logger "github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
)

// --- Fake core fetchers ---

type fakeListingFetcher struct {
	fn func(ctx context.Context, companyID string) (core.JobListing, error)
}

func (f *fakeListingFetcher) FetchJobListing(ctx context.Context, companyID string) (core.JobListing, error) {
	return f.fn(ctx, companyID)
}

type fakeConfigListingFetcher struct {
	fn func(ctx context.Context, companyID string, config json.RawMessage) (core.JobListing, error)
}

func (f *fakeConfigListingFetcher) FetchJobListingWithConfig(
	ctx context.Context, companyID string, config json.RawMessage,
) (core.JobListing, error) {
	return f.fn(ctx, companyID, config)
}

// --- Mock OrchestratorClient ---

type mockBatchClient struct {
	batches []PageBatch
	err     error
}

func (m *mockBatchClient) GetWork(_ context.Context, _ []string) (*GetWorkResult, error) {
	return &GetWorkResult{}, nil
}

func (m *mockBatchClient) SubmitBatch(_ context.Context, batch PageBatch) error {
	m.batches = append(m.batches, batch)
	return m.err
}

func (m *mockBatchClient) ReportError(_ context.Context, _ WorkError) error { return nil }

// --- Helpers ---

// makeExecutor wires an executor whose lookup always returns the given bundle.
func makeExecutor(t *testing.T, bundle adapters.AdapterBundle, client OrchestratorClient) AdapterExecutor {
	t.Helper()
	lookup := func(_ string, _ logger.Logger, _ *http.Client) (adapters.AdapterBundle, error) {
		return bundle, nil
	}
	return newAdapterExecutor(client, lookup, &http.Client{}, logger.NewNoop())
}

func workUnit(adapterType string) WorkUnit {
	return WorkUnit{
		AssignmentID: "assign-1",
		CompanyID:    "company-1",
		AdapterType:  adapterType,
	}
}

// --- Tests ---

// 1. Thin/standard adapter → FetchJobListing returns summaries → client
// receives []core.JobSummary with Final=true.
func TestAdapterExecutor_Listing_SubmitsSingleFinalBatch(t *testing.T) {
	jobs := []core.JobSummary{
		{ID: "lever-abc", Title: "SWE", URL: "https://example.com", Location: "Remote"},
	}
	bundle := adapters.AdapterBundle{
		Listing: &fakeListingFetcher{fn: func(_ context.Context, _ string) (core.JobListing, error) {
			return core.JobListing{Jobs: jobs}, nil
		}},
	}
	client := &mockBatchClient{}
	exec := makeExecutor(t, bundle, client)

	if err := exec.Execute(context.Background(), workUnit("lever")); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if len(client.batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(client.batches))
	}
	b := client.batches[0]
	if !b.Final {
		t.Error("expected Final=true on single batch")
	}
	if len(b.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(b.Jobs))
	}
	if b.Jobs[0].ID != "lever-abc" {
		t.Errorf("job ID = %q, want lever-abc", b.Jobs[0].ID)
	}
	if b.Jobs[0].Location != "Remote" {
		t.Errorf("job Location = %q, want Remote", b.Jobs[0].Location)
	}
	if b.AssignmentID != "assign-1" {
		t.Errorf("AssignmentID = %q, want assign-1", b.AssignmentID)
	}
	if b.CompanyID != "company-1" {
		t.Errorf("CompanyID = %q, want company-1", b.CompanyID)
	}
	if b.AdapterType != "lever" {
		t.Errorf("AdapterType = %q, want lever", b.AdapterType)
	}
	if b.FetchedCount != 1 {
		t.Errorf("FetchedCount = %d, want 1", b.FetchedCount)
	}
}

// 2. Configurable adapter with AdapterConfig set → FetchJobListingWithConfig is
// called with the config bytes; Listing is NOT used.
func TestAdapterExecutor_Configurable_UsesWithConfig(t *testing.T) {
	var receivedConfig json.RawMessage
	bundle := adapters.AdapterBundle{
		Listing: &fakeListingFetcher{fn: func(_ context.Context, _ string) (core.JobListing, error) {
			t.Error("FetchJobListing should not be called when config is present")
			return core.JobListing{}, nil
		}},
		ConfigListing: &fakeConfigListingFetcher{fn: func(_ context.Context, _ string, cfg json.RawMessage) (core.JobListing, error) {
			receivedConfig = cfg
			return core.JobListing{Jobs: []core.JobSummary{{ID: "wd-1"}}}, nil
		}},
	}
	client := &mockBatchClient{}
	exec := makeExecutor(t, bundle, client)

	cfg := `{"host":"acme.wd3.myworkdayjobs.com"}`
	unit := workUnit("workday")
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

// 2b. Configurable adapter present but no config supplied → falls through to
// Listing (which, for a real configurable adapter, returns a directing error).
func TestAdapterExecutor_Configurable_NilConfig_UsesListing(t *testing.T) {
	bundle := adapters.AdapterBundle{
		Listing: &fakeListingFetcher{fn: func(_ context.Context, _ string) (core.JobListing, error) {
			return core.JobListing{}, errors.New("requires config; use FetchJobListingWithConfig")
		}},
		ConfigListing: &fakeConfigListingFetcher{fn: func(_ context.Context, _ string, _ json.RawMessage) (core.JobListing, error) {
			t.Error("FetchJobListingWithConfig should not be called when config is nil")
			return core.JobListing{}, nil
		}},
	}
	client := &mockBatchClient{}
	exec := makeExecutor(t, bundle, client)

	if err := exec.Execute(context.Background(), workUnit("workday")); err == nil {
		t.Error("expected error surfaced from Listing, got nil")
	}
	if len(client.batches) != 0 {
		t.Errorf("expected no submit on listing error, got %d batches", len(client.batches))
	}
}

// 3. Listing returns error → executor returns error, no submit.
func TestAdapterExecutor_FetchError_ReturnsError_NoSubmit(t *testing.T) {
	bundle := adapters.AdapterBundle{
		Listing: &fakeListingFetcher{fn: func(_ context.Context, _ string) (core.JobListing, error) {
			return core.JobListing{}, errors.New("fetch failed")
		}},
	}
	client := &mockBatchClient{}
	exec := makeExecutor(t, bundle, client)

	if err := exec.Execute(context.Background(), workUnit("lever")); err == nil {
		t.Error("expected error for fetch failure, got nil")
	}
	if len(client.batches) != 0 {
		t.Errorf("expected no submit on fetch error, got %d batches", len(client.batches))
	}
}

// 3b. Configurable fetch returns error → executor returns error, no submit.
func TestAdapterExecutor_ConfigFetchError_ReturnsError_NoSubmit(t *testing.T) {
	bundle := adapters.AdapterBundle{
		Listing: &fakeListingFetcher{fn: func(_ context.Context, _ string) (core.JobListing, error) {
			return core.JobListing{}, nil
		}},
		ConfigListing: &fakeConfigListingFetcher{fn: func(_ context.Context, _ string, _ json.RawMessage) (core.JobListing, error) {
			return core.JobListing{}, errors.New("bad config")
		}},
	}
	client := &mockBatchClient{}
	exec := makeExecutor(t, bundle, client)

	cfg := `{"host":"x"}`
	unit := workUnit("workday")
	unit.AdapterConfig = &cfg

	if err := exec.Execute(context.Background(), unit); err == nil {
		t.Error("expected error for config fetch failure, got nil")
	}
	if len(client.batches) != 0 {
		t.Errorf("expected no submit on config fetch error, got %d batches", len(client.batches))
	}
}

// 4. Empty listing → submits Jobs: [] with Final=true.
func TestAdapterExecutor_EmptyListing_SubmitsEmptyFinalBatch(t *testing.T) {
	bundle := adapters.AdapterBundle{
		Listing: &fakeListingFetcher{fn: func(_ context.Context, _ string) (core.JobListing, error) {
			return core.JobListing{Jobs: nil}, nil
		}},
	}
	client := &mockBatchClient{}
	exec := makeExecutor(t, bundle, client)

	if err := exec.Execute(context.Background(), workUnit("lever")); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if len(client.batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(client.batches))
	}
	b := client.batches[0]
	if !b.Final {
		t.Error("expected Final=true")
	}
	if len(b.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(b.Jobs))
	}
	if b.FetchedCount != 0 {
		t.Errorf("FetchedCount = %d, want 0", b.FetchedCount)
	}
}

// 5. Unknown adapter type → error wrapping adapters.ErrInvalidAdapter, no submit.
func TestAdapterExecutor_UnknownAdapter_ReturnsErrInvalidAdapter(t *testing.T) {
	lookup := func(_ string, _ logger.Logger, _ *http.Client) (adapters.AdapterBundle, error) {
		return adapters.AdapterBundle{}, adapters.ErrInvalidAdapter
	}
	client := &mockBatchClient{}
	exec := newAdapterExecutor(client, lookup, &http.Client{}, logger.NewNoop())

	err := exec.Execute(context.Background(), workUnit("badtype"))
	if err == nil {
		t.Fatal("expected error for unknown adapter type, got nil")
	}
	if !errors.Is(err, adapters.ErrInvalidAdapter) {
		t.Errorf("error %v does not wrap ErrInvalidAdapter", err)
	}
	if len(client.batches) != 0 {
		t.Errorf("expected no submit on unknown adapter, got %d batches", len(client.batches))
	}
}

// SubmitBatch error → executor surfaces it.
func TestAdapterExecutor_SubmitBatchError_ReturnsError(t *testing.T) {
	bundle := adapters.AdapterBundle{
		Listing: &fakeListingFetcher{fn: func(_ context.Context, _ string) (core.JobListing, error) {
			return core.JobListing{Jobs: []core.JobSummary{{ID: "j1"}}}, nil
		}},
	}
	client := &mockBatchClient{err: errors.New("network error")}
	exec := makeExecutor(t, bundle, client)

	if err := exec.Execute(context.Background(), workUnit("lever")); err == nil {
		t.Error("expected error from SubmitBatch, got nil")
	}
}

// Default lookup (adapters.New) resolves a real registry adapter and surfaces
// ErrInvalidAdapter for unknown types — exercises the production seam.
func TestAdapterExecutor_DefaultLookup_UnknownAdapter(t *testing.T) {
	client := &mockBatchClient{}
	exec := newAdapterExecutor(client, adapters.New, &http.Client{}, logger.NewNoop())

	err := exec.Execute(context.Background(), workUnit("does-not-exist"))
	if err == nil {
		t.Fatal("expected error for unknown adapter via default lookup")
	}
	if !errors.Is(err, adapters.ErrInvalidAdapter) {
		t.Errorf("error %v does not wrap ErrInvalidAdapter", err)
	}
}

// Successful Execute increments the success scan counter and the jobs-fetched counter.
func TestExecute_IncrementsSuccessMetrics(t *testing.T) {
	beforeScans := testutil.ToFloat64(metrics.CrawlerScansTotal.WithLabelValues("success"))
	beforeFetched := testutil.ToFloat64(metrics.CrawlerJobsFetchedTotal)

	bundle := adapters.AdapterBundle{
		Listing: &fakeListingFetcher{fn: func(_ context.Context, _ string) (core.JobListing, error) {
			return core.JobListing{Jobs: []core.JobSummary{{ID: "j1"}, {ID: "j2"}}}, nil
		}},
	}
	client := &mockBatchClient{}
	exec := makeExecutor(t, bundle, client)

	if err := exec.Execute(context.Background(), workUnit("lever")); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if got := testutil.ToFloat64(metrics.CrawlerScansTotal.WithLabelValues("success")) - beforeScans; got != 1 {
		t.Errorf("CrawlerScansTotal{success}: want +1, got +%g", got)
	}
	if got := testutil.ToFloat64(metrics.CrawlerJobsFetchedTotal) - beforeFetched; got != 2 {
		t.Errorf("CrawlerJobsFetchedTotal: want +2, got +%g", got)
	}
}

// A lookup error increments the error scan counter.
func TestExecute_IncrementsErrorMetric(t *testing.T) {
	beforeErrors := testutil.ToFloat64(metrics.CrawlerScansTotal.WithLabelValues("error"))

	lookupFn := func(_ string, _ logger.Logger, _ *http.Client) (adapters.AdapterBundle, error) {
		return adapters.AdapterBundle{}, errors.New("adapter not found")
	}
	client := &mockBatchClient{}
	exec := newAdapterExecutor(client, lookupFn, &http.Client{}, logger.NewNoop())
	_ = exec.Execute(context.Background(), workUnit("unknown"))

	if got := testutil.ToFloat64(metrics.CrawlerScansTotal.WithLabelValues("error")) - beforeErrors; got != 1 {
		t.Errorf("CrawlerScansTotal{error}: want +1, got +%g", got)
	}
}
