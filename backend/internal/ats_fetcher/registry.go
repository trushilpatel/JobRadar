package ats_fetcher

import (
	"fmt"

	"github.com/trushilpatel/jobradar/internal/ats_fetcher/ashby"
	"github.com/trushilpatel/jobradar/internal/ats_fetcher/chromedp"
	"github.com/trushilpatel/jobradar/internal/ats_fetcher/greenhouse"
	"github.com/trushilpatel/jobradar/internal/ats_fetcher/lever"
	"github.com/trushilpatel/jobradar/internal/ats_fetcher/workable"
	"github.com/trushilpatel/jobradar/internal/ats_fetcher/workday"
	"github.com/trushilpatel/jobradar/internal/model"
)

// Registry returns the correct Adapter for a given adapter_type string.
// The returned value may also implement ContentFetcher — callers should
// type-assert to check (see job_post_analyzer).
func Registry(adapterType string) (Adapter, error) {
	switch adapterType {
	case model.AdapterGreenhouse:
		return &greenhouse.Greenhouse{}, nil
	case model.AdapterLever:
		return &lever.Lever{}, nil
	case model.AdapterAshby:
		return &ashby.Ashby{}, nil
	case model.AdapterWorkday:
		return &workday.Workday{}, nil
	case model.AdapterWorkable:
		return &workable.Workable{}, nil
	case model.AdapterChromedp:
		return &chromedp.Chromedp{}, nil
	default:
		return nil, fmt.Errorf("unknown adapter type: %s", adapterType)
	}
}
