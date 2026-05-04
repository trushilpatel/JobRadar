package tests

import (
	"context"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/platform/cache"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
)

// countingRepo counts how many times each Service method is invoked. Used
// to prove that the cache layer absorbs reads on hits and re-issues them
// on misses or after invalidation.
type countingRepo struct {
	getCalls    int
	updateCalls int
	deleteCalls int
	user        *usermodel.User
	updateErr   error
	deleteErr   error
}

func (c *countingRepo) Get(_ context.Context, _ string) (*usermodel.User, error) {
	c.getCalls++
	if c.user == nil {
		return nil, nil
	}
	cp := *c.user
	return &cp, nil
}
func (c *countingRepo) GetByEmail(_ context.Context, _ string) (*usermodel.User, error) {
	return nil, nil
}
func (c *countingRepo) Provision(_ context.Context, _, _ string) (*usermodel.User, error) {
	return nil, nil
}
func (c *countingRepo) UpdateProfile(_ context.Context, _, _ string) error {
	c.updateCalls++
	return c.updateErr
}
func (c *countingRepo) Delete(_ context.Context, _ string) error {
	c.deleteCalls++
	return c.deleteErr
}

const cacheTTL = 30 * time.Second

func TestCacheService_HitsAndMisses(t *testing.T) {
	t.Parallel()
	repo := &countingRepo{user: &usermodel.User{ID: "u1", Email: "a@b.com"}}
	c := cache.NewMemoryCache()
	s := usersvc.NewCacheService(repo, c, cacheTTL)
	ctx := context.Background()

	// Miss → repo.
	if _, err := s.Get(ctx, "u1"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if repo.getCalls != 1 {
		t.Fatalf("repo.getCalls = %d, want 1", repo.getCalls)
	}
	// Hit → cache.
	if _, err := s.Get(ctx, "u1"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if repo.getCalls != 1 {
		t.Fatalf("repo.getCalls after hit = %d, want still 1", repo.getCalls)
	}
}

func TestCacheService_InvalidatesOnUpdate(t *testing.T) {
	t.Parallel()
	repo := &countingRepo{user: &usermodel.User{ID: "u1", Email: "a@b.com"}}
	c := cache.NewMemoryCache()
	s := usersvc.NewCacheService(repo, c, cacheTTL)
	ctx := context.Background()

	// Populate cache.
	if _, err := s.Get(ctx, "u1"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	// Update invalidates.
	if err := s.UpdateProfile(ctx, "u1", "Alice"); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	// Next Get hits the repo again.
	if _, err := s.Get(ctx, "u1"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if repo.getCalls != 2 {
		t.Errorf("repo.getCalls after update = %d, want 2", repo.getCalls)
	}
}

func TestCacheService_InvalidatesOnDelete(t *testing.T) {
	t.Parallel()
	repo := &countingRepo{user: &usermodel.User{ID: "u1", Email: "a@b.com"}}
	c := cache.NewMemoryCache()
	s := usersvc.NewCacheService(repo, c, cacheTTL)
	ctx := context.Background()

	if _, err := s.Get(ctx, "u1"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if err := s.Delete(ctx, "u1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// Confirm both keys are gone.
	var dest usermodel.User
	hit, err := c.Get(ctx, "user:row:u1", &dest)
	if err != nil {
		t.Fatalf("cache get: %v", err)
	}
	if hit {
		t.Error("user:row:u1 cache key not invalidated")
	}
	hit, err = c.Get(ctx, "user:u1", &dest)
	if err != nil {
		t.Fatalf("cache get: %v", err)
	}
	if hit {
		t.Error("user:u1 identity cache key not invalidated")
	}
}

func TestCacheService_NilCacheIsPassthrough(t *testing.T) {
	t.Parallel()
	repo := &countingRepo{user: &usermodel.User{ID: "u1", Email: "a@b.com"}}
	s := usersvc.NewCacheService(repo, nil, cacheTTL)
	ctx := context.Background()

	if _, err := s.Get(ctx, "u1"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := s.Get(ctx, "u1"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if repo.getCalls != 2 {
		t.Errorf("with nil cache, every call must reach the repo; got %d", repo.getCalls)
	}
}
