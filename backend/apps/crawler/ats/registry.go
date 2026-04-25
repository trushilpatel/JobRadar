package ats

import (
	"fmt"

	"github.com/go-rod/rod"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/apple"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/ashby"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/google"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/greenhouse"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/lever"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/meta"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/microsoft"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/netflix"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/rippling"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/uber"
	"github.com/trushilpatel/jobradar/common/logger"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
)

// Registry returns the correct Adapter for a given adapter_type string.
// The returned value may also implement ContentFetcher — callers should
// type-assert to check (see job_post_analyzer).
// For the Google adapter, use RegistryWithGoogleConfig to pass scraper settings.
func Registry(adapterType string) (Adapter, error) {
	return RegistryWithGoogleConfig(adapterType, google.Config{}, logger.NewNoop())
}

// SetBrowser calls WithBrowser on rod-based adapters (Google, Meta) if they support it.
// HTTP-only adapters are left untouched. Safe to call with a nil browser.
func SetBrowser(adp Adapter, b *rod.Browser) {
	type browserSetter interface{ WithBrowser(*rod.Browser) }
	if setter, ok := adp.(browserSetter); ok {
		setter.WithBrowser(b)
	}
}

// RegistryWithGoogleConfig is like Registry but passes scraper config and a logger
// to the Google adapter. For all other adapter types googleCfg and log are ignored.
func RegistryWithGoogleConfig(adapterType string, googleCfg google.Config, log logger.Logger) (Adapter, error) {
	switch adapterType {
	case companycommon.AdapterGreenhouse:
		return &greenhouse.Greenhouse{}, nil
	case companycommon.AdapterLever:
		return &lever.Lever{}, nil
	case companycommon.AdapterAshby:
		return &ashby.Ashby{}, nil
	case companycommon.AdapterGoogle:
		if googleCfg == (google.Config{}) {
			return google.NewRodGoogleDefault(), nil
		}
		return google.NewRodGoogle(googleCfg, log), nil
	case companycommon.AdapterNetflix:
		return netflix.New(), nil
	case companycommon.AdapterUber:
		return uber.New(), nil
	case companycommon.AdapterMicrosoft:
		return microsoft.New(), nil
	case companycommon.AdapterRippling:
		return rippling.New(), nil
	case companycommon.AdapterApple:
		return apple.New(), nil
	case companycommon.AdapterMeta:
		return meta.NewRodMetaDefault(), nil
	default:
		return nil, fmt.Errorf("unknown adapter type: %s", adapterType)
	}
}
