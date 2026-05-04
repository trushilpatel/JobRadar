package cache_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/platform/cache"
)

type sampleValue struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestMemoryCache_HitMissSetDelete(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()

	// Miss on empty cache.
	var dest sampleValue
	hit, err := c.Get(ctx, "k", &dest)
	if err != nil {
		t.Fatalf("Get on empty cache: unexpected err %v", err)
	}
	if hit {
		t.Fatal("Get on empty cache: want hit=false")
	}

	// Hit after Set.
	in := sampleValue{Name: "alice", Count: 7}
	if err := c.Set(ctx, "k", in, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	hit, err = c.Get(ctx, "k", &dest)
	if err != nil {
		t.Fatalf("Get after Set: %v", err)
	}
	if !hit {
		t.Fatal("Get after Set: want hit=true")
	}
	if dest != in {
		t.Fatalf("Get after Set: got %+v want %+v", dest, in)
	}

	// Delete removes the entry.
	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	dest = sampleValue{}
	hit, _ = c.Get(ctx, "k", &dest)
	if hit {
		t.Fatal("Get after Delete: want hit=false")
	}

	// Delete on missing key is idempotent.
	if err := c.Delete(ctx, "missing"); err != nil {
		t.Fatalf("Delete missing: %v", err)
	}
}

func TestMemoryCache_Expiry(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()

	// Negative TTL ⇒ no expiry (per contract). Use a tiny TTL and sleep
	// briefly to verify expiry fires.
	if err := c.Set(ctx, "k", sampleValue{Name: "v"}, 5*time.Millisecond); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Hit immediately.
	var dest sampleValue
	hit, _ := c.Get(ctx, "k", &dest)
	if !hit {
		t.Fatal("immediate Get: want hit=true")
	}

	time.Sleep(20 * time.Millisecond)

	hit, _ = c.Get(ctx, "k", &dest)
	if hit {
		t.Fatal("Get after expiry: want hit=false")
	}
}

func TestGetOrLoad_LoaderRunsOnceUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()

	const goroutines = 32
	var calls int32

	loader := func(_ context.Context) (sampleValue, error) {
		// Simulate slow load so concurrent goroutines pile up on
		// singleflight rather than each completing before the next runs.
		atomic.AddInt32(&calls, 1)
		time.Sleep(20 * time.Millisecond)
		return sampleValue{Name: "loaded", Count: 1}, nil
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			v, err := cache.GetOrLoad(ctx, c, "shared-key", time.Minute, loader)
			if err != nil {
				t.Errorf("GetOrLoad: %v", err)
				return
			}
			if v.Name != "loaded" || v.Count != 1 {
				t.Errorf("GetOrLoad: unexpected value %+v", v)
			}
		}()
	}
	wg.Wait()

	got := atomic.LoadInt32(&calls)
	if got != 1 {
		t.Fatalf("loader call count: want 1 (singleflight), got %d", got)
	}

	// Subsequent calls hit the cache (no new loads).
	v, err := cache.GetOrLoad(ctx, c, "shared-key", time.Minute, loader)
	if err != nil {
		t.Fatalf("GetOrLoad post-cache: %v", err)
	}
	if v.Name != "loaded" {
		t.Fatalf("GetOrLoad post-cache: %+v", v)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatal("loader ran again after cache hit — singleflight/cache write-back broken")
	}
}

func TestGetOrLoad_LoaderErrorPropagates(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemoryCache()

	wantErr := errString("nope")
	_, err := cache.GetOrLoad(ctx, c, "k", time.Minute, func(_ context.Context) (sampleValue, error) {
		return sampleValue{}, wantErr
	})
	if err == nil {
		t.Fatal("GetOrLoad with failing loader: want err, got nil")
	}
}

func TestNoopCache_AlwaysMisses(t *testing.T) {
	ctx := context.Background()
	c := cache.NewNoopCache()

	if err := c.Set(ctx, "k", sampleValue{Name: "x"}, time.Minute); err != nil {
		t.Fatalf("NoopCache.Set: %v", err)
	}
	var dest sampleValue
	hit, err := c.Get(ctx, "k", &dest)
	if err != nil {
		t.Fatalf("NoopCache.Get: %v", err)
	}
	if hit {
		t.Fatal("NoopCache.Get: want hit=false after Set")
	}
	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatalf("NoopCache.Delete: %v", err)
	}
}

// errString is a tiny error type used for table-style error assertions
// without pulling in the stdlib errors package's New just for tests.
type errString string

func (e errString) Error() string { return string(e) }
