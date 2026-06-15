package adapters

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/apple"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/ashby"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/deel"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/eightfold"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/gem"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/google"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/greenhouse"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/lever"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/meta"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/microsoft"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/netflix"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/rippling"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/uber"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/workday"
	"github.com/trushilpatel/jobradar/common/ats/core"
	"github.com/trushilpatel/jobradar/common/logger"
)

// ErrInvalidAdapter is returned when an adapter type string does not map to
// any registered adapter in the registry.
var ErrInvalidAdapter = errors.New("crawler node: invalid adapter")

// AdapterBundle groups the interfaces an adapter may implement.
// At least one of Listing or ConfigListing is always non-nil.
// Post is nil for fat adapters (Gem, Uber, Deel) that populate JobContent
// in FetchJobListing so a separate post-fetch step is not needed.
// ConfigListing is nil for non-configurable adapters.
type AdapterBundle struct {
	Listing       core.ListingFetcher
	Post          core.PostFetcher
	ConfigListing core.ConfigurableListingFetcher
}

// Constructor builds an AdapterBundle for the given logger and HTTP client.
type Constructor func(log logger.Logger, httpClient *http.Client) AdapterBundle

// Registry returns a map from adapter type name to its Constructor.
// Keys match the `ats.name` primary key in the database.
// Adding a new adapter = new package + one entry here.
func Registry() map[string]Constructor {
	return map[string]Constructor{
		"apple": func(log logger.Logger, c *http.Client) AdapterBundle {
			a := apple.New(log, c)
			return AdapterBundle{Listing: a, Post: a}
		},
		"ashby": func(log logger.Logger, c *http.Client) AdapterBundle {
			a := ashby.New(log, c)
			return AdapterBundle{Listing: a, Post: a}
		},
		"deel": func(log logger.Logger, c *http.Client) AdapterBundle {
			a := deel.New(log, c)
			return AdapterBundle{Listing: a}
		},
		"eightfold": func(log logger.Logger, c *http.Client) AdapterBundle {
			a := eightfold.New(log, c)
			return AdapterBundle{Listing: a, Post: a, ConfigListing: a}
		},
		"gem": func(log logger.Logger, c *http.Client) AdapterBundle {
			a := gem.New(log, c)
			return AdapterBundle{Listing: a}
		},
		"google": func(log logger.Logger, c *http.Client) AdapterBundle {
			a := google.New(log, c)
			return AdapterBundle{Listing: a, Post: a}
		},
		"greenhouse": func(log logger.Logger, c *http.Client) AdapterBundle {
			a := greenhouse.New(log, c)
			return AdapterBundle{Listing: a, Post: a}
		},
		"lever": func(log logger.Logger, c *http.Client) AdapterBundle {
			a := lever.New(log, c)
			return AdapterBundle{Listing: a, Post: a}
		},
		"meta": func(log logger.Logger, c *http.Client) AdapterBundle {
			a := meta.New(log, c)
			return AdapterBundle{Listing: a, Post: a}
		},
		"microsoft": func(log logger.Logger, c *http.Client) AdapterBundle {
			a := microsoft.New(log, c)
			return AdapterBundle{Listing: a, Post: a}
		},
		"netflix": func(log logger.Logger, c *http.Client) AdapterBundle {
			a := netflix.New(log, c)
			return AdapterBundle{Listing: a, Post: a}
		},
		"rippling": func(log logger.Logger, c *http.Client) AdapterBundle {
			a := rippling.New(log, c)
			return AdapterBundle{Listing: a, Post: a}
		},
		"uber": func(log logger.Logger, c *http.Client) AdapterBundle {
			a := uber.New(log, c)
			return AdapterBundle{Listing: a}
		},
		"workday": func(log logger.Logger, c *http.Client) AdapterBundle {
			a := workday.New(log, c)
			return AdapterBundle{Listing: a, Post: a, ConfigListing: a}
		},
	}
}

// New constructs an AdapterBundle for the given adapter type name.
// Returns ErrInvalidAdapter (wrapped) if the type is not registered.
func New(adapterType string, log logger.Logger, httpClient *http.Client) (AdapterBundle, error) {
	ctor, ok := Registry()[adapterType]
	if !ok {
		return AdapterBundle{}, fmt.Errorf("adapters.New %q: %w", adapterType, ErrInvalidAdapter)
	}
	return ctor(log, httpClient), nil
}
