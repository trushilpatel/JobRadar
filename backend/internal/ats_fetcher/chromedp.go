package ats_fetcher

import (
	"context"
	"fmt"
)

type Chromedp struct{}

func (c *Chromedp) FetchJobs(ctx context.Context, companyURL string) (FetchJobsResponse, error) {
	// TODO: implement Chromedp adapter
	return FetchJobsResponse{}, fmt.Errorf("chromedp: not implemented")
}
