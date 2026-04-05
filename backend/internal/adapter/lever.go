package adapter

import "context"

type Lever struct{}

func (l *Lever) FetchJobs(ctx context.Context, companyURL string) ([]RawJob, error) {
	// TODO: implement Lever adapter
	return nil, nil
}
