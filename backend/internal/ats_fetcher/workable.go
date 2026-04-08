package ats_fetcher

import (
	"context"
	"fmt"
)

type Workable struct{}

func (w *Workable) FetchJobs(ctx context.Context, companyURL string) (FetchJobsResponse, error) {
	// TODO: implement Workable adapter
	return FetchJobsResponse{}, fmt.Errorf("workable: not implemented")
}
