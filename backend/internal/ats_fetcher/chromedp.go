package ats_fetcher

import "context"

type Chromedp struct{}

func (c *Chromedp) FetchJobs(ctx context.Context, companyURL string) ([]RawJob, error) {
	// TODO: implement Chromedp adapter (headless browser for JS-rendered pages)
	return nil, nil
}
