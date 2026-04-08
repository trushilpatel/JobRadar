package ats_fetcher

import (
	"context"
	"fmt"
)

type Workday struct{}

func (w *Workday) FetchJobs(ctx context.Context, companyURL string) (FetchJobsResponse, error) {
	// TODO: implement Workday adapter
	return FetchJobsResponse{}, fmt.Errorf("workday: not implemented")
}
