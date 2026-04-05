package adapter

import "context"

type Workday struct{}

func (w *Workday) FetchJobs(ctx context.Context, companyURL string) ([]RawJob, error) {
	// TODO: implement Workday adapter
	return nil, nil
}
