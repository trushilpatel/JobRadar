package tests

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/platform/cache"
	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
)

const atsCacheTTL = 30 * time.Second

// countingAtsNext counts invocations to assert cache hit/miss behaviour.
type countingAtsNext struct {
	mu           sync.Mutex
	listCalls    int
	getCalls     map[string]int
	updateCalls  int
	listResult   []atsdto.ATSDTO
	listErr      error
	getResult    map[string]atsdto.ATSDTO
	getErr       error
	updateResult atsdto.ATSDTO
	updateErr    error
}

func newCountingAtsNext() *countingAtsNext {
	return &countingAtsNext{
		getCalls:  make(map[string]int),
		getResult: make(map[string]atsdto.ATSDTO),
	}
}

func (c *countingAtsNext) ListATS(_ context.Context) ([]atsdto.ATSDTO, error) {
	c.mu.Lock()
	c.listCalls++
	c.mu.Unlock()
	return c.listResult, c.listErr
}

func (c *countingAtsNext) GetATS(_ context.Context, name string) (atsdto.ATSDTO, error) {
	c.mu.Lock()
	c.getCalls[name]++
	c.mu.Unlock()
	if r, ok := c.getResult[name]; ok {
		return r, c.getErr
	}
	return atsdto.ATSDTO{}, c.getErr
}

func (c *countingAtsNext) UpdateATS(_ context.Context, _ string, _ atsdto.UpdateATSInput) (atsdto.ATSDTO, error) {
	c.mu.Lock()
	c.updateCalls++
	c.mu.Unlock()
	return c.updateResult, c.updateErr
}

// ---------- ListATS cache hit/miss ----------

func TestAtsCacheService_ListATS_CacheMissCallsNext(t *testing.T) {
	t.Parallel()
	next := newCountingAtsNext()
	next.listResult = []atsdto.ATSDTO{{Name: "greenhouse"}}
	c := cache.NewMemoryCache()
	svc := atssvc.NewCacheService(next, c, atsCacheTTL)
	ctx := context.Background()

	items, err := svc.ListATS(ctx)
	if err != nil {
		t.Fatalf("ListATS: %v", err)
	}
	if next.listCalls != 1 {
		t.Errorf("listCalls = %d, want 1", next.listCalls)
	}
	if len(items) != 1 || items[0].Name != "greenhouse" {
		t.Errorf("items = %v, want [greenhouse]", items)
	}
}

func TestAtsCacheService_ListATS_CacheHitDoesNotCallNext(t *testing.T) {
	t.Parallel()
	next := newCountingAtsNext()
	next.listResult = []atsdto.ATSDTO{{Name: "lever"}}
	c := cache.NewMemoryCache()
	svc := atssvc.NewCacheService(next, c, atsCacheTTL)
	ctx := context.Background()

	// First call populates cache.
	if _, err := svc.ListATS(ctx); err != nil {
		t.Fatalf("first ListATS: %v", err)
	}
	// Second call should hit cache.
	if _, err := svc.ListATS(ctx); err != nil {
		t.Fatalf("second ListATS: %v", err)
	}
	if next.listCalls != 1 {
		t.Errorf("listCalls = %d after cache hit, want still 1", next.listCalls)
	}
}

// ---------- GetATS cache hit/miss ----------

func TestAtsCacheService_GetATS_CacheMissCallsNext(t *testing.T) {
	t.Parallel()
	next := newCountingAtsNext()
	next.getResult["ashby"] = atsdto.ATSDTO{Name: "ashby", DisplayName: "Ashby"}
	c := cache.NewMemoryCache()
	svc := atssvc.NewCacheService(next, c, atsCacheTTL)
	ctx := context.Background()

	got, err := svc.GetATS(ctx, "ashby")
	if err != nil {
		t.Fatalf("GetATS: %v", err)
	}
	if next.getCalls["ashby"] != 1 {
		t.Errorf("getCalls[ashby] = %d, want 1", next.getCalls["ashby"])
	}
	if got.Name != "ashby" {
		t.Errorf("name = %q, want ashby", got.Name)
	}
}

func TestAtsCacheService_GetATS_CacheHitDoesNotCallNext(t *testing.T) {
	t.Parallel()
	next := newCountingAtsNext()
	next.getResult["lever"] = atsdto.ATSDTO{Name: "lever"}
	c := cache.NewMemoryCache()
	svc := atssvc.NewCacheService(next, c, atsCacheTTL)
	ctx := context.Background()

	if _, err := svc.GetATS(ctx, "lever"); err != nil {
		t.Fatalf("first GetATS: %v", err)
	}
	if _, err := svc.GetATS(ctx, "lever"); err != nil {
		t.Fatalf("second GetATS: %v", err)
	}
	if next.getCalls["lever"] != 1 {
		t.Errorf("getCalls[lever] = %d after cache hit, want still 1", next.getCalls["lever"])
	}
}

// ---------- UpdateATS cache invalidation ----------

func TestAtsCacheService_UpdateATS_InvalidatesBothKeys(t *testing.T) {
	t.Parallel()
	next := newCountingAtsNext()
	next.listResult = []atsdto.ATSDTO{{Name: "greenhouse"}}
	next.getResult["greenhouse"] = atsdto.ATSDTO{Name: "greenhouse"}
	next.updateResult = atsdto.ATSDTO{Name: "greenhouse", IsActive: false}
	c := cache.NewMemoryCache()
	svc := atssvc.NewCacheService(next, c, atsCacheTTL)
	ctx := context.Background()

	// Populate both cache keys.
	if _, err := svc.ListATS(ctx); err != nil {
		t.Fatalf("ListATS: %v", err)
	}
	if _, err := svc.GetATS(ctx, "greenhouse"); err != nil {
		t.Fatalf("GetATS: %v", err)
	}

	// Update should invalidate both.
	active := false
	if _, err := svc.UpdateATS(ctx, "greenhouse", atsdto.UpdateATSInput{IsActive: &active}); err != nil {
		t.Fatalf("UpdateATS: %v", err)
	}

	// Next reads must hit the source again.
	if _, err := svc.ListATS(ctx); err != nil {
		t.Fatalf("ListATS after update: %v", err)
	}
	if _, err := svc.GetATS(ctx, "greenhouse"); err != nil {
		t.Fatalf("GetATS after update: %v", err)
	}
	if next.listCalls != 2 {
		t.Errorf("listCalls = %d, want 2 (pre+post update)", next.listCalls)
	}
	if next.getCalls["greenhouse"] != 2 {
		t.Errorf("getCalls[greenhouse] = %d, want 2", next.getCalls["greenhouse"])
	}
}

// ---------- Nil cache / ttl≤0 passthrough ----------

func TestAtsCacheService_NilCache_Passthrough(t *testing.T) {
	t.Parallel()
	next := newCountingAtsNext()
	next.listResult = []atsdto.ATSDTO{{Name: "lever"}}
	svc := atssvc.NewCacheService(next, nil, atsCacheTTL)
	ctx := context.Background()

	if _, err := svc.ListATS(ctx); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := svc.ListATS(ctx); err != nil {
		t.Fatalf("second: %v", err)
	}
	if next.listCalls != 2 {
		t.Errorf("listCalls = %d with nil cache, want 2", next.listCalls)
	}
}

func TestAtsCacheService_ZeroTTL_Passthrough(t *testing.T) {
	t.Parallel()
	next := newCountingAtsNext()
	next.listResult = []atsdto.ATSDTO{{Name: "lever"}}
	c := cache.NewMemoryCache()
	svc := atssvc.NewCacheService(next, c, 0)
	ctx := context.Background()

	if _, err := svc.ListATS(ctx); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := svc.ListATS(ctx); err != nil {
		t.Fatalf("second: %v", err)
	}
	if next.listCalls != 2 {
		t.Errorf("listCalls = %d with ttl=0, want 2", next.listCalls)
	}
}

func TestAtsCacheService_GetATS_ZeroTTL_Passthrough(t *testing.T) {
	t.Parallel()
	next := newCountingAtsNext()
	next.getResult["lever"] = atsdto.ATSDTO{Name: "lever"}
	c := cache.NewMemoryCache()
	svc := atssvc.NewCacheService(next, c, 0)
	ctx := context.Background()

	if _, err := svc.GetATS(ctx, "lever"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := svc.GetATS(ctx, "lever"); err != nil {
		t.Fatalf("second: %v", err)
	}
	if next.getCalls["lever"] != 2 {
		t.Errorf("getCalls[lever] = %d with ttl=0, want 2", next.getCalls["lever"])
	}
}

// ---------- UpdateATS error propagation ----------

func TestAtsCacheService_UpdateATS_PropagatesError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("update failed")
	next := newCountingAtsNext()
	next.updateErr = wantErr
	c := cache.NewMemoryCache()
	svc := atssvc.NewCacheService(next, c, atsCacheTTL)

	active := true
	_, err := svc.UpdateATS(context.Background(), "lever", atsdto.UpdateATSInput{IsActive: &active})
	if !errors.Is(err, wantErr) {
		t.Errorf("UpdateATS err = %v, want %v", err, wantErr)
	}
}

// ---------- Error propagation on miss ----------

// When the downstream returns an error on a cache miss, the error must be
// returned verbatim and the cache must not be populated with garbage.
func TestAtsCacheService_ListATS_PropagatesDownstreamError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("downstream list error")
	next := newCountingAtsNext()
	next.listErr = wantErr
	c := cache.NewMemoryCache()
	svc := atssvc.NewCacheService(next, c, atsCacheTTL)

	_, err := svc.ListATS(context.Background())
	if !errors.Is(err, wantErr) {
		t.Errorf("ListATS err = %v, want %v", err, wantErr)
	}
	// Cache must not be populated on error.
	var dest []atsdto.ATSDTO
	hit, _ := c.Get(context.Background(), "ats:list", &dest)
	if hit {
		t.Error("cache populated after downstream error")
	}
}

// Same test for GetATS.
func TestAtsCacheService_GetATS_PropagatesDownstreamError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("downstream get error")
	next := newCountingAtsNext()
	next.getErr = wantErr
	c := cache.NewMemoryCache()
	svc := atssvc.NewCacheService(next, c, atsCacheTTL)

	_, err := svc.GetATS(context.Background(), "lever")
	if !errors.Is(err, wantErr) {
		t.Errorf("GetATS err = %v, want %v", err, wantErr)
	}
	var dest atsdto.ATSDTO
	hit, _ := c.Get(context.Background(), "ats:lever", &dest)
	if hit {
		t.Error("cache populated after downstream error")
	}
}

// ---------- Race safety ----------

func TestAtsCacheService_ConcurrentReads_NoDataRace(t *testing.T) {
	t.Parallel()
	next := newCountingAtsNext()
	next.listResult = []atsdto.ATSDTO{{Name: "greenhouse"}, {Name: "lever"}}
	c := cache.NewMemoryCache()
	svc := atssvc.NewCacheService(next, c, atsCacheTTL)
	ctx := context.Background()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = svc.ListATS(ctx)
		}()
	}
	wg.Wait()
	// No assertion beyond "didn't panic / data race". The race detector
	// enforces this when the test is run with -race.
}
