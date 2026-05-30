package tests

import (
	"context"
	"errors"
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
func (c *countingRepo) GetConfig(_ context.Context, _ string) (*usermodel.UserConfig, error) {
	return &usermodel.UserConfig{}, nil
}
func (c *countingRepo) PatchConfig(_ context.Context, _ string, partial usermodel.UserConfig) (*usermodel.UserConfig, error) {
	return &partial, nil
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

// ttl <= 0 must turn Get into a pass-through too — caches with no expiry
// would risk indefinite stale reads, so the decorator explicitly disables
// itself rather than emitting an immortal cache entry.
func TestCacheService_ZeroTTLIsPassthrough(t *testing.T) {
	t.Parallel()
	repo := &countingRepo{user: &usermodel.User{ID: "u1", Email: "a@b.com"}}
	c := cache.NewMemoryCache()
	s := usersvc.NewCacheService(repo, c, 0)
	ctx := context.Background()
	if _, err := s.Get(ctx, "u1"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := s.Get(ctx, "u1"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if repo.getCalls != 2 {
		t.Errorf("with ttl<=0, every Get must reach the repo; got %d", repo.getCalls)
	}
	// Cache must NOT have been populated.
	var dest usermodel.User
	hit, err := c.Get(ctx, "user:row:u1", &dest)
	if err != nil {
		t.Fatalf("cache get: %v", err)
	}
	if hit {
		t.Error("cache populated despite ttl <= 0")
	}
}

// Nil cache must NOT break UpdateProfile / Delete — they must continue to
// delegate to next and propagate its error verbatim.
func TestCacheService_NilCacheDelegatesWritesAndPropagatesErrors(t *testing.T) {
	t.Parallel()
	repo := &countingRepo{
		user:      &usermodel.User{ID: "u1"},
		updateErr: errors.New("update boom"),
		deleteErr: errors.New("delete boom"),
	}
	s := usersvc.NewCacheService(repo, nil, cacheTTL)
	ctx := context.Background()

	if err := s.UpdateProfile(ctx, "u1", "Alice"); err == nil || err.Error() != "update boom" {
		t.Errorf("UpdateProfile err = %v, want 'update boom'", err)
	}
	if repo.updateCalls != 1 {
		t.Errorf("update calls = %d, want 1", repo.updateCalls)
	}
	if err := s.Delete(ctx, "u1"); err == nil || err.Error() != "delete boom" {
		t.Errorf("Delete err = %v, want 'delete boom'", err)
	}
	if repo.deleteCalls != 1 {
		t.Errorf("delete calls = %d, want 1", repo.deleteCalls)
	}
}

// On a Get miss with a downstream error, the cache must NOT swallow the
// error or populate the cache with a zero value — both would silently
// corrupt the contract.
func TestCacheService_GetPropagatesDownstreamErrorAndDoesNotCache(t *testing.T) {
	t.Parallel()
	repo := &countingRepo{user: nil} // returns (nil, nil) — but we want an error
	repo.user = nil
	wantErr := errors.New("repo down")
	repoFake := &errorRepo{err: wantErr}
	c := cache.NewMemoryCache()
	s := usersvc.NewCacheService(repoFake, c, cacheTTL)
	ctx := context.Background()

	if _, err := s.Get(ctx, "u1"); !errors.Is(err, wantErr) {
		t.Errorf("Get err = %v, want %v", err, wantErr)
	}
	var dest usermodel.User
	hit, err := c.Get(ctx, "user:row:u1", &dest)
	if err != nil {
		t.Fatalf("cache get: %v", err)
	}
	if hit {
		t.Error("cache populated after downstream error; want clean miss")
	}
}

// UpdateProfile must propagate the next.UpdateProfile error verbatim and skip
// the post-write evict (otherwise an error path could mask a stale cache as
// "all-good").
func TestCacheService_UpdateProfilePropagatesError(t *testing.T) {
	t.Parallel()
	repo := &countingRepo{user: &usermodel.User{ID: "u1"}, updateErr: errors.New("boom")}
	c := cache.NewMemoryCache()
	s := usersvc.NewCacheService(repo, c, cacheTTL)
	ctx := context.Background()

	if err := s.UpdateProfile(ctx, "u1", "Alice"); err == nil {
		t.Fatal("expected error from next.UpdateProfile")
	}
}

// Delete must propagate the next.Delete error verbatim — same reasoning.
func TestCacheService_DeletePropagatesError(t *testing.T) {
	t.Parallel()
	repo := &countingRepo{user: &usermodel.User{ID: "u1"}, deleteErr: errors.New("boom")}
	c := cache.NewMemoryCache()
	s := usersvc.NewCacheService(repo, c, cacheTTL)
	ctx := context.Background()
	if err := s.Delete(ctx, "u1"); err == nil {
		t.Fatal("expected error from next.Delete")
	}
}

// errorRepo is a minimal Service that always returns the configured error
// from Get. Used to exercise the cache layer's miss-then-error branch.
type errorRepo struct {
	err error
}

func (e *errorRepo) Get(_ context.Context, _ string) (*usermodel.User, error) {
	return nil, e.err
}
func (e *errorRepo) GetByEmail(_ context.Context, _ string) (*usermodel.User, error) {
	return nil, e.err
}
func (e *errorRepo) Provision(_ context.Context, _, _ string) (*usermodel.User, error) {
	return nil, e.err
}
func (e *errorRepo) UpdateProfile(_ context.Context, _, _ string) error { return e.err }
func (e *errorRepo) Delete(_ context.Context, _ string) error           { return e.err }
func (e *errorRepo) GetConfig(_ context.Context, _ string) (*usermodel.UserConfig, error) {
	return nil, e.err
}
func (e *errorRepo) PatchConfig(_ context.Context, _ string, _ usermodel.UserConfig) (*usermodel.UserConfig, error) {
	return nil, e.err
}
