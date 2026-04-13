package ats

import (
	"fmt"

	"github.com/trushilpatel/jobradar/pipeline/ats/ashby"
	"github.com/trushilpatel/jobradar/pipeline/ats/google"
	"github.com/trushilpatel/jobradar/pipeline/ats/greenhouse"
	"github.com/trushilpatel/jobradar/pipeline/ats/lever"
	"github.com/trushilpatel/jobradar/model"
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
	case model.AdapterGoogle:
		if googleCfg == (google.Config{}) {
			return google.NewDefault(), nil
		}
		return google.New(googleCfg), nil
	default:
		return nil, fmt.Errorf("unknown adapter type: %s", adapterType)
	}
}
