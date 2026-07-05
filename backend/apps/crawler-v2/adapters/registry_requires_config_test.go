package adapters_test

import (
	"net/http"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/service/ats/adaptermeta"
)

// TestRegistry_RequiresConfigMatchesConfigListing is the single guard
// against adaptermeta.RequiresConfig and registry.go's ConfigListing wiring
// ever drifting apart. It derives the "needs config" set directly from the
// registry (every adapter name whose AdapterBundle has a non-nil
// ConfigListing) rather than hardcoding a second copy of the adapter list,
// so adding or removing a configurable adapter in registry.go without
// updating adaptermeta immediately fails this test.
func TestRegistry_RequiresConfigMatchesConfigListing(t *testing.T) {
	log := logger.NewNoop()
	client := &http.Client{}
	reg := adapters.Registry()

	for name, ctor := range reg {
		bundle := ctor(log, client)
		wantRequiresConfig := bundle.ConfigListing != nil
		gotRequiresConfig := adaptermeta.RequiresConfig(name)
		if gotRequiresConfig != wantRequiresConfig {
			t.Errorf("adapter %q: registry ConfigListing non-nil = %v, adaptermeta.RequiresConfig = %v — these must match",
				name, wantRequiresConfig, gotRequiresConfig)
		}
	}
}
