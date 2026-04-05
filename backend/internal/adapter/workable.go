package adapter

import "context"

type Workable struct{}

func (w *Workable) FetchJobs(ctx context.Context, companyURL string) ([]RawJob, error) {
	// TODO: implement Workable adapter
	return nil, nil
}
