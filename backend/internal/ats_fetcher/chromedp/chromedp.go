package chromedp

import (
	"context"
	"fmt"

	"github.com/trushilpatel/jobradar/internal/ats_fetcher/types"
)

type Chromedp struct{}

func (c *Chromedp) FetchJobs(ctx context.Context, companyURL string) (types.FetchJobsResponse, error) {
	// TODO: implement Chromedp adapter
	return types.FetchJobsResponse{}, fmt.Errorf("chromedp: not implemented")
}
