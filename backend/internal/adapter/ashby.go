package adapter

import "context"

type Ashby struct{}

func (a *Ashby) FetchJobs(ctx context.Context, companyURL string) ([]RawJob, error) {
	// TODO: implement Ashby adapter
	return nil, nil
}
