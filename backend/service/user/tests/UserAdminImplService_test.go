package tests

import (
	"context"
	"errors"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/platform/cache"
	"github.com/trushilpatel/jobradar/platform/clerkadmin"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
)

// fakeAdminRepo is a hand-rolled AdminService implementation. The admin impl
// layer only orchestrates calls to next + the clerk client + cache, so a
// recording fake here is sufficient to assert the orchestration order and
// error propagation rules.
type fakeAdminRepo struct {
	listCalled   bool
	listResult   usersvc.AdminListResult
	listErr      error
	getCalled    bool
	getResult    *usersvc.UserWithRoles
	getErr       error
	setRolesArgs struct {
		id        string
		roleIDs   []string
		grantedBy string
	}
	setRolesCalled bool
	setRolesErr    error
	setStatusArgs  struct {
		id, status string
	}
	setStatusCalled bool
	setStatusErr    error
	deleteCalled    bool
	deleteID        string
	deleteErr       error
}

func (f *fakeAdminRepo) List(_ context.Context, _ usersvc.AdminUserFilter, _, _ int) (usersvc.AdminListResult, error) {
	f.listCalled = true
	return f.listResult, f.listErr
}
func (f *fakeAdminRepo) GetWithRoles(_ context.Context, _ string) (*usersvc.UserWithRoles, error) {
	f.getCalled = true
	return f.getResult, f.getErr
}
func (f *fakeAdminRepo) SetRoles(_ context.Context, id string, roleIDs []string, grantedBy string) error {
	f.setRolesCalled = true
	f.setRolesArgs.id = id
	f.setRolesArgs.roleIDs = roleIDs
	f.setRolesArgs.grantedBy = grantedBy
	return f.setRolesErr
}
func (f *fakeAdminRepo) SetStatus(_ context.Context, id, status string) error {
	f.setStatusCalled = true
	f.setStatusArgs.id = id
	f.setStatusArgs.status = status
	return f.setStatusErr
}
func (f *fakeAdminRepo) AdminDelete(_ context.Context, id string) error {
	f.deleteCalled = true
	f.deleteID = id
	return f.deleteErr
}

// ---------- SetRoles ----------

func TestAdminImpl_SetRoles_EvictsIdentityOnSuccess(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	c := cache.NewMemoryCache()
	ctx := context.Background()
	// Pre-populate the identity cache so we can assert the post-write evict.
	if err := c.Set(ctx, "user:u1", map[string]string{"k": "v"}, 0); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	svc := usersvc.NewAdminImplService(repo, nil, c)

	if err := svc.SetRoles(ctx, "u1", []string{"admin"}, "actor"); err != nil {
		t.Fatalf("SetRoles: %v", err)
	}
	if !repo.setRolesCalled {
		t.Fatal("repo.SetRoles not invoked")
	}
	var dest map[string]string
	hit, err := c.Get(ctx, "user:u1", &dest)
	if err != nil {
		t.Fatalf("cache get: %v", err)
	}
	if hit {
		t.Error("identity cache key not evicted after SetRoles")
	}
}

func TestAdminImpl_SetRoles_DoesNotEvictOnError(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{setRolesErr: errors.New("db down")}
	c := cache.NewMemoryCache()
	ctx := context.Background()
	if err := c.Set(ctx, "user:u1", map[string]string{"k": "v"}, 0); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := usersvc.NewAdminImplService(repo, nil, c)

	if err := svc.SetRoles(ctx, "u1", nil, "actor"); err == nil {
		t.Fatal("expected error from repo")
	}
	var dest map[string]string
	hit, err := c.Get(ctx, "user:u1", &dest)
	if err != nil {
		t.Fatalf("cache get: %v", err)
	}
	if !hit {
		t.Error("identity cache evicted on failed SetRoles; expected to remain")
	}
}

func TestAdminImpl_SetRoles_NilCacheIsNoOp(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	svc := usersvc.NewAdminImplService(repo, nil, nil)
	if err := svc.SetRoles(context.Background(), "u1", []string{"admin"}, "actor"); err != nil {
		t.Fatalf("SetRoles: %v", err)
	}
	if !repo.setRolesCalled {
		t.Fatal("repo.SetRoles not invoked")
	}
}

// ---------- SetStatus ----------

func TestAdminImpl_SetStatus_EvictsIdentityOnSuccess(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	c := cache.NewMemoryCache()
	ctx := context.Background()
	if err := c.Set(ctx, "user:u1", "anything", 0); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := usersvc.NewAdminImplService(repo, nil, c)

	if err := svc.SetStatus(ctx, "u1", "suspended"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if !repo.setStatusCalled {
		t.Fatal("repo.SetStatus not invoked")
	}
	var dest string
	hit, err := c.Get(ctx, "user:u1", &dest)
	if err != nil {
		t.Fatalf("cache get: %v", err)
	}
	if hit {
		t.Error("identity cache key not evicted after SetStatus")
	}
}

func TestAdminImpl_SetStatus_DoesNotEvictOnError(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{setStatusErr: errors.New("db down")}
	c := cache.NewMemoryCache()
	ctx := context.Background()
	if err := c.Set(ctx, "user:u1", "anything", 0); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := usersvc.NewAdminImplService(repo, nil, c)

	if err := svc.SetStatus(ctx, "u1", "suspended"); err == nil {
		t.Fatal("expected error")
	}
	var dest string
	hit, err := c.Get(ctx, "user:u1", &dest)
	if err != nil {
		t.Fatalf("cache get: %v", err)
	}
	if !hit {
		t.Error("identity cache evicted on failed SetStatus; expected to remain")
	}
}

// ---------- AdminDelete ----------

// fakeAdminClerk is reused from UserImplService tests where fakeClerk
// exists, but the AdminImpl chain expects the same Client interface so
// declaring a tiny dedicated fake keeps blast radius low.
type fakeAdminClerk struct {
	called bool
	id     string
	err    error
}

func (f *fakeAdminClerk) DeleteUser(_ context.Context, id string) error {
	f.called = true
	f.id = id
	return f.err
}

func TestAdminImpl_AdminDelete_OrderingClerkThenDBThenCache(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	sb := &fakeAdminClerk{}
	c := cache.NewMemoryCache()
	ctx := context.Background()
	if err := c.Set(ctx, "user:u1", "anything", 0); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := usersvc.NewAdminImplService(repo, sb, c)

	if err := svc.AdminDelete(ctx, "u1"); err != nil {
		t.Fatalf("AdminDelete: %v", err)
	}
	if !sb.called {
		t.Error("clerk DeleteUser not invoked")
	}
	if !repo.deleteCalled {
		t.Error("repo.AdminDelete not invoked")
	}
	if sb.id != "u1" || repo.deleteID != "u1" {
		t.Errorf("user ids = clerk %q, repo %q; want u1", sb.id, repo.deleteID)
	}
	var dest string
	hit, _ := c.Get(ctx, "user:u1", &dest)
	if hit {
		t.Error("identity cache not evicted post-delete")
	}
}

func TestAdminImpl_AdminDelete_EmptyID(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	svc := usersvc.NewAdminImplService(repo, &fakeAdminClerk{}, nil)
	err := svc.AdminDelete(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty id")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrBadRequest.Code {
		t.Errorf("code = %q, want BAD_REQUEST", se.Code)
	}
	if repo.deleteCalled {
		t.Error("repo should not be called when id is empty")
	}
}

func TestAdminImpl_AdminDelete_AbortsOnClerkFailure(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	sb := &fakeAdminClerk{err: errors.New("clerk 500")}
	svc := usersvc.NewAdminImplService(repo, sb, nil)
	err := svc.AdminDelete(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected clerk failure to propagate")
	}
	if repo.deleteCalled {
		t.Error("repo.AdminDelete must not run after clerk failure")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrInternal.Code {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// If the Clerk account is already gone (e.g. a prior delete attempt's Clerk
// call succeeded but the DB delete step afterward failed, or a retried
// delete), clerkadmin.ErrUserNotFound must be treated as "already deleted,
// proceed" — not as a fatal error. Otherwise the local `users` row is
// permanently orphaned: the UI's only delete action always fails with a
// hard error and offers no way to remove that row afterward.
func TestAdminImpl_AdminDelete_ProceedsWhenClerkUserAlreadyGone(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	sb := &fakeAdminClerk{err: clerkadmin.ErrUserNotFound}
	c := cache.NewMemoryCache()
	ctx := context.Background()
	if err := c.Set(ctx, "user:u1", "anything", 0); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := usersvc.NewAdminImplService(repo, sb, c)

	if err := svc.AdminDelete(ctx, "u1"); err != nil {
		t.Fatalf("AdminDelete must succeed when the Clerk user is already gone: %v", err)
	}
	if !repo.deleteCalled {
		t.Error("repo.AdminDelete must still run when Clerk returns ErrUserNotFound")
	}
	if repo.deleteID != "u1" {
		t.Errorf("repo delete id = %q, want u1", repo.deleteID)
	}
	var dest string
	if hit, _ := c.Get(ctx, "user:u1", &dest); hit {
		t.Error("identity cache must still be evicted")
	}
}

// A genuine Clerk failure (not "already gone") must still abort before the
// DB delete — only ErrUserNotFound is tolerated.
func TestAdminImpl_AdminDelete_StillAbortsOnRealClerkFailure(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	sb := &fakeAdminClerk{err: errors.New("clerk 500")}
	svc := usersvc.NewAdminImplService(repo, sb, nil)
	err := svc.AdminDelete(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected non-404 clerk failure to still propagate")
	}
	if repo.deleteCalled {
		t.Error("repo.AdminDelete must not run after a real clerk failure")
	}
}

// When the clerk client is nil (test/dev), the impl layer must still
// invoke the repo. Otherwise admin deletes would silently no-op in tests.
func TestAdminImpl_AdminDelete_NilClerkDelegatesToRepo(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	svc := usersvc.NewAdminImplService(repo, nil, nil)
	if err := svc.AdminDelete(context.Background(), "u1"); err != nil {
		t.Fatalf("AdminDelete: %v", err)
	}
	if !repo.deleteCalled {
		t.Error("repo.AdminDelete not invoked when clerk is nil")
	}
}

// When the repo delete fails after clerk succeeds, the cache must NOT be
// evicted — we leave the stale identity bundle alone so the caller has the
// option to retry without invalidating active sessions on a half-failed
// delete.
func TestAdminImpl_AdminDelete_RepoFailureSkipsCacheEvict(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{deleteErr: errors.New("db down")}
	sb := &fakeAdminClerk{}
	c := cache.NewMemoryCache()
	ctx := context.Background()
	if err := c.Set(ctx, "user:u1", "anything", 0); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := usersvc.NewAdminImplService(repo, sb, c)
	if err := svc.AdminDelete(ctx, "u1"); err == nil {
		t.Fatal("expected repo error")
	}
	var dest string
	hit, err := c.Get(ctx, "user:u1", &dest)
	if err != nil {
		t.Fatalf("cache get: %v", err)
	}
	if !hit {
		t.Error("cache evicted on failed repo delete; should not have been")
	}
}

// ---------- Delegation through BaseAdminService ----------

// List and GetWithRoles aren't overridden by adminImplService — they should
// pass through to next unchanged.
func TestAdminImpl_DelegatesListAndGet(t *testing.T) {
	t.Parallel()
	want := &usersvc.UserWithRoles{Roles: []string{"admin"}}
	repo := &fakeAdminRepo{
		listResult: usersvc.AdminListResult{Total: 42},
		getResult:  want,
	}
	svc := usersvc.NewAdminImplService(repo, nil, nil)

	if _, err := svc.List(context.Background(), usersvc.AdminUserFilter{}, 1, 10); err != nil {
		t.Fatalf("List: %v", err)
	}
	if !repo.listCalled {
		t.Error("repo.List not invoked")
	}
	got, err := svc.GetWithRoles(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetWithRoles: %v", err)
	}
	if got != want {
		t.Errorf("GetWithRoles returned %v, want %v", got, want)
	}
}

// NewBaseAdminService must panic when next is nil — that guards against
// a misconfigured wire setup.
func TestAdminImpl_NilNextPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil next")
		}
	}()
	_ = usersvc.NewAdminImplService(nil, nil, nil)
}
