package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ats "github.com/trushilpatel/jobradar/apps/crawler/ats"
	google "github.com/trushilpatel/jobradar/apps/crawler/ats/google"
	"github.com/trushilpatel/jobradar/common/logger"
)

// adapterLookupFn resolves an adapter by type name. Injectable for tests.
type adapterLookupFn func(adapterType string, googleCfg google.Config, log logger.Logger) (ats.Adapter, error)

type adapterExecutor struct {
	client       OrchestratorClient
	lookupFn     adapterLookupFn
	googleConfig google.Config
	log          logger.Logger
}

// newAdapterExecutor constructs an AdapterExecutor that wires the ATS adapter
// interfaces to the OrchestratorClient HTTP callbacks.
func newAdapterExecutor(
	client OrchestratorClient,
	lookupFn adapterLookupFn,
	googleConfig google.Config,
	log logger.Logger,
) AdapterExecutor {
	return &adapterExecutor{
		client:       client,
		lookupFn:     lookupFn,
		googleConfig: googleConfig,
		log:          log,
	}
}

// Execute runs one WorkUnit end-to-end:
//  1. Resolves the ATS adapter
//  2. Wires optional adapter interfaces to HTTP callbacks
//  3. Fetches the job listing
//  4. Submits the result(s) to the orchestrator
func (e *adapterExecutor) Execute(ctx context.Context, unit WorkUnit) error {
	adp, err := e.lookupFn(unit.AdapterType, e.googleConfig, e.log)
	if err != nil {
		return fmt.Errorf("executor: lookup adapter %q: %w", unit.AdapterType, err)
	}

	knownSet := toIDSet(unit.KnownIDs)

	// KnownJobsProvider: inject pre-scan snapshot to skip already-known posts.
	if kp, ok := adp.(ats.KnownJobsProvider); ok {
		kp.SetKnownJobIDs(knownSet)
	}

	// SeenChecker: O(1) lookup against the pre-scan snapshot — no HTTP required.
	if sc, ok := adp.(ats.SeenChecker); ok {
		sc.SetSeenChecker(func(_ context.Context, ids []string) (map[string]bool, error) {
			result := make(map[string]bool, len(ids))
			for _, id := range ids {
				_, result[id] = knownSet[id]
			}
			return result, nil
		})
	}

	start := time.Now()
	totalFetched := 0
	streamed := false

	// StreamPublisher: forward each page to the orchestrator without buffering.
	if sp, ok := adp.(ats.StreamPublisher); ok {
		streamed = true
		sp.SetStreamPublish(func(items []ats.FetchJobsItem) {
			totalFetched += len(items)
			batch := PageBatch{
				AssignmentID:     unit.AssignmentID,
				CompanyID:        unit.CompanyID,
				AdapterType:      unit.AdapterType,
				Jobs:             mapJobs(items),
				PreviousKnownIDs: unit.KnownIDs,
				FetchedCount:     len(items),
				DurationMs:       time.Since(start).Milliseconds(),
				Final:            false,
			}
			if submitErr := e.client.SubmitBatch(ctx, batch); submitErr != nil {
				e.log.Error("streaming batch submit failed", submitErr)
			}
		})
	}

	var result ats.FetchJobsResponse
	if ca, ok := adp.(ats.ConfigurableAdapter); ok && unit.AdapterConfig != nil {
		result, err = ca.FetchJobsWithConfig(ctx, unit.CompanyID, json.RawMessage(*unit.AdapterConfig))
	} else {
		result, err = adp.FetchJobs(ctx, unit.CompanyID)
	}
	if err != nil {
		return fmt.Errorf("executor: fetch %q: %w", unit.AdapterType, err)
	}

	if streamed {
		// Terminal signal: marks the assignment complete on the orchestrator.
		return e.client.SubmitBatch(ctx, PageBatch{
			AssignmentID:     unit.AssignmentID,
			CompanyID:        unit.CompanyID,
			AdapterType:      unit.AdapterType,
			Jobs:             nil,
			PreviousKnownIDs: unit.KnownIDs,
			FetchedCount:     totalFetched,
			DurationMs:       time.Since(start).Milliseconds(),
			Final:            true,
		})
	}

	return e.client.SubmitBatch(ctx, PageBatch{
		AssignmentID:     unit.AssignmentID,
		CompanyID:        unit.CompanyID,
		AdapterType:      unit.AdapterType,
		Jobs:             mapJobs(result.Jobs),
		PreviousKnownIDs: unit.KnownIDs,
		FetchedCount:     len(result.Jobs),
		DurationMs:       time.Since(start).Milliseconds(),
		Final:            true,
	})
}

func mapJobs(items []ats.FetchJobsItem) []CrawledJob {
	jobs := make([]CrawledJob, len(items))
	for i, item := range items {
		jobs[i] = CrawledJob{
			ID:             item.ID,
			Title:          item.Title,
			URL:            item.URL,
			Location:       item.Location,
			EmploymentType: item.EmploymentType,
			LocationKnown:  item.LocationKnown,
			AllLocations:   item.AllLocations,
			FullContent:    mapJobContent(item.FullContent),
		}
	}
	return jobs
}

func mapJobContent(fc *ats.FullJobContent) *CrawledJobContent {
	if fc == nil {
		return nil
	}
	return &CrawledJobContent{
		Description:  fc.Description,
		Requirements: fc.Requirements,
		Salary:       fc.Compensation,
		Department:   fc.Department,
	}
}

func toIDSet(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}
