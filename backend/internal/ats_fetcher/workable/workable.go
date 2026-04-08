package workable

import (
	"context"
	"fmt"

	"github.com/trushilpatel/jobradar/internal/ats_fetcher/types"
)

type Workable struct{}

func (w *Workable) FetchJobs(ctx context.Context, companyURL string) (types.FetchJobsResponse, error) {
	// TODO: implement Workable adapter
	return types.FetchJobsResponse{}, fmt.Errorf("workable: not implemented")
}
