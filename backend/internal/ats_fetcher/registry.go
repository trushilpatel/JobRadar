package ats_fetcher

import (
	"fmt"

	"github.com/trushilpatel/jobradar/internal/ats_fetcher/ashby"
	"github.com/trushilpatel/jobradar/internal/ats_fetcher/chromedp"
	"github.com/trushilpatel/jobradar/internal/ats_fetcher/google"
	"github.com/trushilpatel/jobradar/internal/ats_fetcher/greenhouse"
	"github.com/trushilpatel/jobradar/internal/ats_fetcher/lever"
	"github.com/trushilpatel/jobradar/internal/ats_fetcher/workable"
	"github.com/trushilpatel/jobradar/internal/ats_fetcher/workday"
	"github.com/trushilpatel/jobradar/internal/model"
)

// Registry returns the correct Adapter for a given adapter_type string.
// The returned value may also implement ContentFetcher — callers should
// type-assert to check (see job_post_analyzer).
// For the Google adapter, use RegistryWithGoogleConfig to pass scraper settings.
func Registry(adapterType string) (Adapter, error) {
	return RegistryWithGoogleConfig(adapterType, google.Config{})
}

// RegistryWithGoogleConfig is like Registry but passes scraper config to the
// Google adapter. For all other adapter types the googleCfg is ignored.
func RegistryWithGoogleConfig(adapterType string, googleCfg google.Config) (Adapter, error) {
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
	case model.AdapterGoogle:
		if googleCfg == (google.Config{}) {
			return google.NewDefault(), nil
		}
		return google.New(googleCfg), nil
	default:
		return nil, fmt.Errorf("unknown adapter type: %s", adapterType)
	}
}
