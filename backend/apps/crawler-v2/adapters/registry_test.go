package adapters_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters"
	"github.com/trushilpatel/jobradar/common/logger"
)

var allAdapterTypes = []string{
	"apple", "ashby", "deel", "eightfold", "gem",
	"google", "greenhouse", "lever", "meta", "microsoft",
	"netflix", "rippling", "uber", "workday",
}

func TestRegistry_ContainsAll14Adapters(t *testing.T) {
	reg := adapters.Registry()
	for _, typ := range allAdapterTypes {
		if _, ok := reg[typ]; !ok {
			t.Errorf("Registry() missing adapter type %q", typ)
		}
	}
	if got, want := len(reg), len(allAdapterTypes); got != want {
		t.Errorf("Registry() len = %d, want %d", got, want)
	}
}

func TestRegistry_EachConstructorReturnsValidBundle(t *testing.T) {
	log := logger.NewNoop()
	client := &http.Client{}
	reg := adapters.Registry()

	for _, typ := range allAdapterTypes {
		ctor := reg[typ]
		bundle := ctor(log, client)
		// Every adapter must expose at least a ListingFetcher or ConfigurableListingFetcher.
		if bundle.Listing == nil && bundle.ConfigListing == nil {
			t.Errorf("%s: bundle has neither Listing nor ConfigListing", typ)
		}
	}
}

func TestNew_ReturnsErrInvalidAdapterForUnknownType(t *testing.T) {
	_, err := adapters.New("nonexistent-ats", logger.NewNoop(), &http.Client{})
	if err == nil {
		t.Fatal("expected error for unknown adapter type, got nil")
	}
	if !isErrInvalidAdapter(err) {
		t.Errorf("expected ErrInvalidAdapter in error chain, got: %v", err)
	}
}

func isErrInvalidAdapter(err error) bool {
	return errors.Is(err, adapters.ErrInvalidAdapter)
}

func TestNew_ReturnsValidBundleForKnownType(t *testing.T) {
	bundle, err := adapters.New("greenhouse", logger.NewNoop(), &http.Client{})
	if err != nil {
		t.Fatalf("New(greenhouse) error = %v, want nil", err)
	}
	if bundle.Listing == nil {
		t.Error("New(greenhouse) bundle.Listing is nil")
	}
	if bundle.Post == nil {
		t.Error("New(greenhouse) bundle.Post is nil")
	}
}
