package cache

// local_internal_test.go — white-box tests for defensive branches in LocalCache
// that are unreachable through the public API:
//
//  1. takeFresh: the guard that removes a list element whose Value is not *localEntry
//  2. putWithTTL: the fall-through when an existing key's element holds a non-*localEntry
//
// These branches exist to prevent panics from unexpected in-process corruption;
// testing them requires direct access to the unexported fields (mu, items, order).

import (
	"container/list"
	"context"
	"testing"
	"time"
)

// injectCorruptElement inserts a list element into the LocalCache's LRU map
// whose Value is a plain string rather than *localEntry. This simulates
// in-process corruption that the defensive branches guard against.
func injectCorruptElement(lc *LocalCache, key string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	el := lc.order.PushFront("not-a-localEntry") // wrong type
	lc.items[key] = el
}

// TestTakeFresh_CorruptElementRemoved verifies that takeFresh removes a list
// element whose Value is not *localEntry and returns (nil, false) so the
// caller falls through to the wrapped cache.
func TestTakeFresh_CorruptElementRemoved(t *testing.T) {
	lc, ok := NewLocalCache(NewMemoryCache(), time.Minute, 10).(*LocalCache)
	if !ok {
		t.Fatal("NewLocalCache must return *LocalCache when TTL+size are positive")
	}

	injectCorruptElement(lc, "bad")

	data, hit := lc.takeFresh("bad")
	if hit {
		t.Fatal("takeFresh with corrupt element: want hit=false")
	}
	if data != nil {
		t.Fatalf("takeFresh with corrupt element: want nil data, got %v", data)
	}

	// The corrupt element must have been cleaned up.
	lc.mu.Lock()
	_, still := lc.items["bad"]
	lc.mu.Unlock()
	if still {
		t.Fatal("takeFresh must remove the corrupt element from the map")
	}
}

// TestPutWithTTL_CorruptExistingEntryCreatesNew verifies that putWithTTL
// creates a new *localEntry when the existing element for the same key holds
// a non-*localEntry value, rather than panicking.
func TestPutWithTTL_CorruptExistingEntryCreatesNew(t *testing.T) {
	lc, ok := NewLocalCache(NewMemoryCache(), time.Minute, 10).(*LocalCache)
	if !ok {
		t.Fatal("NewLocalCache must return *LocalCache when TTL+size are positive")
	}

	// Inject a corrupt element for key "k".
	injectCorruptElement(lc, "k")
	originalLen := lc.order.Len()

	// putWithTTL must not panic and must create a valid new entry.
	data := []byte(`"value"`)
	lc.putWithTTL("k", data, time.Minute)

	lc.mu.Lock()
	el, ok := lc.items["k"]
	lc.mu.Unlock()
	if !ok {
		t.Fatal("putWithTTL: key missing after write")
	}
	entry, ok := el.Value.(*localEntry)
	if !ok {
		t.Fatalf("putWithTTL: stored element is not *localEntry, got %T", el.Value)
	}
	if string(entry.data) != string(data) {
		t.Fatalf("putWithTTL: data mismatch: want %s got %s", data, entry.data)
	}
	// List grew by one (old corrupt element + new entry), capped by maxSize.
	// With maxSize=10 and only 2 elements, both should be present.
	if lc.order.Len() != originalLen+1 {
		t.Fatalf("list length after putWithTTL with corrupt entry: want %d, got %d",
			originalLen+1, lc.order.Len())
	}
}

// TestEvict_RemovesKeyFromMapAndList verifies evict clears the specified key
// from both the items map and the LRU list, and is idempotent on missing keys.
func TestEvict_RemovesKeyFromMapAndList(t *testing.T) {
	lc, ok := NewLocalCache(NewMemoryCache(), time.Minute, 10).(*LocalCache)
	if !ok {
		t.Fatal("NewLocalCache must return *LocalCache when TTL+size are positive")
	}

	// Populate via the internal field so we can measure evict independently.
	lc.mu.Lock()
	entry := &localEntry{key: "k", data: []byte(`"v"`), expiresAt: time.Now().Add(time.Minute)}
	el := lc.order.PushFront(entry)
	lc.items["k"] = el
	listLenBefore := lc.order.Len()
	lc.mu.Unlock()

	if listLenBefore != 1 {
		t.Fatalf("pre-condition: list must have 1 element, got %d", listLenBefore)
	}

	lc.evict("k")

	lc.mu.Lock()
	_, still := lc.items["k"]
	listLen := lc.order.Len()
	lc.mu.Unlock()

	if still {
		t.Fatal("evict: key must be removed from items map")
	}
	if listLen != 0 {
		t.Fatalf("evict: list must be empty after eviction, got len=%d", listLen)
	}
}

// TestEvict_MissingKeyIsNoop verifies that evict does not panic or mutate
// state when the key is absent.
func TestEvict_MissingKeyIsNoop(t *testing.T) {
	lc, ok := NewLocalCache(NewMemoryCache(), time.Minute, 10).(*LocalCache)
	if !ok {
		t.Fatal("NewLocalCache must return *LocalCache when TTL+size are positive")
	}

	// Must not panic.
	lc.evict("does-not-exist")

	lc.mu.Lock()
	l := lc.order.Len()
	lc.mu.Unlock()
	if l != 0 {
		t.Fatalf("evict on missing key mutated list: len=%d", l)
	}
}

// TestLocalCache_Get_CorruptLocalEntryFallsThrough tests the blackbox-reachable
// path in Get where a valid locally-cached JSON blob cannot be unmarshalled
// into the caller's dest type. This causes evict() to be called and the get
// to fall through to the wrapped cache.
func TestLocalCache_Get_CorruptLocalEntryFallsThrough(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryCache()
	// Seed the wrapped cache with a string value.
	if err := mem.Set(ctx, "k", "hello", time.Minute); err != nil {
		t.Fatalf("seed: %v", err)
	}

	lc, ok := NewLocalCache(mem, time.Minute, 10).(*LocalCache)
	if !ok {
		t.Fatal("must be *LocalCache")
	}

	// Warm the local layer by fetching as the correct type.
	var s string
	if hit, err := lc.Get(ctx, "k", &s); err != nil || !hit || s != "hello" {
		t.Fatalf("warm get: hit=%v err=%v s=%q", hit, err, s)
	}

	// Now attempt to get the same key with an incompatible type. The local blob
	// is `"hello"` (JSON string) which cannot unmarshal into an int.
	// This must trigger the evict path and fall through to the wrapped cache,
	// which also fails to unmarshal — so we expect an error.
	var n int
	hit, err := lc.Get(ctx, "k", &n)
	if hit {
		t.Fatal("Get with wrong dest: want hit=false")
	}
	if err == nil {
		t.Fatal("Get with wrong dest: want unmarshal error, got nil")
	}

	// Verify the key was evicted from the local layer.
	lc.mu.Lock()
	_, present := lc.items["k"]
	lc.mu.Unlock()
	if present {
		t.Fatal("corrupt local entry must be evicted after Get failure")
	}

	// A subsequent get with the correct type must re-populate from the wrapped cache.
	var s2 string
	hit, err = lc.Get(ctx, "k", &s2)
	if err != nil || !hit || s2 != "hello" {
		t.Fatalf("re-populated get: hit=%v err=%v s=%q", hit, err, s2)
	}

	// And must be back in the local layer now.
	lc.mu.Lock()
	_, present = lc.items["k"]
	lc.mu.Unlock()
	if !present {
		t.Fatal("key must be re-populated in local layer after fallthrough hit")
	}
}

// Dummy reference to prevent "imported and not used" linter errors if list
// is not otherwise used. The list package IS used in the inject helper above.
var _ = list.New
