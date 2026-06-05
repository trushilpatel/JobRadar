package core

import (
	"context"
	"encoding/json"
)

type ListingFetcher interface {
	FetchJobListing(ctx context.Context, companyID string) (JobListing, error)
}

type PostFetcher interface {
	FetchJobPost(ctx context.Context, summary JobSummary) (JobPost, error)
}

// ConfigurableListingFetcher is implemented by adapters that require per-company
// JSON configuration (e.g. Workday needs host+site; Eightfold needs host+domain).
// The engine calls FetchJobListingWithConfig with the raw config JSON bytes;
// the adapter parses and validates it internally.
// FetchJobListing on these adapters always returns an error directing callers here.
type ConfigurableListingFetcher interface {
	FetchJobListingWithConfig(ctx context.Context, companyID string, config json.RawMessage) (JobListing, error)
}
