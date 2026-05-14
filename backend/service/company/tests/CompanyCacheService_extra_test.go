package company_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/platform/cache"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// TestCache_ListPublic_NextErrorPropagates verifies that a failure from the
// underlying chain isn't masked by the cache — the caller must see the error.
func TestCache_ListPublic_NextErrorPropagates(t *testing.T) {
	boom := errors.New("repo failure")
	fake := &fakePublic{err: boom}
	svc := companysvc.NewCacheService(fake, cache.NewMemoryCache(), time.Minute)

	_, err := svc.ListPublic(context.Background(), companydto.CompanyFilter{}, 1, 10)
	if !errors.Is(err, boom) {
		t.Errorf("expected wrapped boom, got %v", err)
	}
}

// TestCache_GetPublic_NextErrorPropagates is the symmetric check for the
// per-id read path.
func TestCache_GetPublic_NextErrorPropagates(t *testing.T) {
	boom := errors.New("repo failure")
	fake := &fakePublic{err: boom}
	svc := companysvc.NewCacheService(fake, cache.NewMemoryCache(), time.Minute)

	_, err := svc.GetPublic(context.Background(), "x")
	if !errors.Is(err, boom) {
		t.Errorf("expected wrapped boom, got %v", err)
	}
}

// TestCache_GetPublic_NilResultIsNotCached locks down the contract that a
// nil view from the chain isn't written into the cache — otherwise a later
// admin-write invalidation pattern wouldn't make sense. The test checks
// behavior by ensuring next is called on every nil-result fetch.
func TestCache_GetPublic_NilResultIsNotCached(t *testing.T) {
	fake := &fakePublic{getResult: nil}
	c := cache.NewMemoryCache()
	svc := companysvc.NewCacheService(fake, c, time.Minute)

	_, _ = svc.GetPublic(context.Background(), "x")
	_, _ = svc.GetPublic(context.Background(), "x")
	if fake.getCalls != 2 {
		t.Errorf("nil result must not be cached; getCalls = %d, want 2", fake.getCalls)
	}
}

// TestCache_ZeroTTLIsPassThrough mirrors the nil-cache guard: ttl<=0 disables
// the decorator entirely.
func TestCache_ZeroTTLIsPassThrough(t *testing.T) {
	fake := &fakePublic{getResult: &companydto.CompanyPublicView{}}
	c := cache.NewMemoryCache()
	svc := companysvc.NewCacheService(fake, c, 0)

	_, _ = svc.GetPublic(context.Background(), "x")
	_, _ = svc.GetPublic(context.Background(), "x")
	if fake.getCalls != 2 {
		t.Errorf("ttl=0 must disable caching; getCalls = %d, want 2", fake.getCalls)
	}

	_, _ = svc.ListPublic(context.Background(), companydto.CompanyFilter{}, 1, 10)
	_, _ = svc.ListPublic(context.Background(), companydto.CompanyFilter{}, 1, 10)
	if fake.listCalls != 2 {
		t.Errorf("ttl=0 must disable list caching; listCalls = %d, want 2", fake.listCalls)
	}
}
