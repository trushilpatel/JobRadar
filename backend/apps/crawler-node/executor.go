package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters"
	"github.com/trushilpatel/jobradar/common/ats/core"
	"github.com/trushilpatel/jobradar/common/logger"
)

// adapterLookupFn resolves an adapter bundle by type name. Injectable for tests.
// The default implementation is adapters.New, which consults the crawler-v2 registry.
type adapterLookupFn func(adapterType string, log logger.Logger, httpClient *http.Client) (adapters.AdapterBundle, error)

type adapterExecutor struct {
	client     OrchestratorClient
	lookupFn   adapterLookupFn
	httpClient *http.Client
	log        logger.Logger
}

// newAdapterExecutor constructs an AdapterExecutor that resolves a crawler-v2
// adapter bundle, runs its listing phase, and submits the result to the
// OrchestratorClient.
func newAdapterExecutor(
	client OrchestratorClient,
	lookupFn adapterLookupFn,
	httpClient *http.Client,
	log logger.Logger,
) AdapterExecutor {
	return &adapterExecutor{
		client:     client,
		lookupFn:   lookupFn,
		httpClient: httpClient,
		log:        log,
	}
}

// Execute runs one WorkUnit end-to-end:
//  1. Resolves the crawler-v2 adapter bundle.
//  2. Runs the LISTING phase only (cheap summaries). Post-fetch is the
//     analyzer's job — crawler-node never calls FetchJobPost.
//  3. Submits the resulting []core.JobSummary to the orchestrator in a single
//     Final batch. The orchestrator stamps CompanyID/Adapter server-side.
func (e *adapterExecutor) Execute(ctx context.Context, unit WorkUnit) error {
	bundle, err := e.lookupFn(unit.AdapterType, e.log, e.httpClient)
	if err != nil {
		return fmt.Errorf("executor: lookup adapter %q: %w", unit.AdapterType, err)
	}

	start := time.Now()

	var listing core.JobListing
	if unit.AdapterConfig != nil && bundle.ConfigListing != nil {
		listing, err = bundle.ConfigListing.FetchJobListingWithConfig(
			ctx, unit.CompanyID, json.RawMessage(*unit.AdapterConfig),
		)
	} else {
		listing, err = bundle.Listing.FetchJobListing(ctx, unit.CompanyID)
	}
	if err != nil {
		return fmt.Errorf("executor: fetch %q: %w", unit.AdapterType, err)
	}

	return e.client.SubmitBatch(ctx, PageBatch{
		AssignmentID: unit.AssignmentID,
		CompanyID:    unit.CompanyID,
		AdapterType:  unit.AdapterType,
		Jobs:         listing.Jobs,
		FetchedCount: len(listing.Jobs),
		DurationMs:   time.Since(start).Milliseconds(),
		Final:        true,
	})
}
