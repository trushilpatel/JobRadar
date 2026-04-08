package workday

import (
	"context"
	"fmt"

	"github.com/trushilpatel/jobradar/internal/ats_fetcher/types"
)

type Workday struct{}

func (w *Workday) FetchJobs(ctx context.Context, companyURL string) (types.FetchJobsResponse, error) {
	// TODO: implement Workday adapter
	return types.FetchJobsResponse{}, fmt.Errorf("workday: not implemented")
}
