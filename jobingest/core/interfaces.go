package core

import "context"

type ListingFetcher interface {
	FetchJobListing(ctx context.Context, companyID string) (JobListing, error)
}

type PostFetcher interface {
	FetchJobPost(ctx context.Context, summary JobSummary) (JobPost, error)
}
