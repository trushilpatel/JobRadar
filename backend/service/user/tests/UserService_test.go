package tests

import (
	"context"
	"errors"
	"testing"

	usermodel "github.com/trushilpatel/jobradar/service/user/models"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
)

// recordingNext is the minimal Service implementation used to verify that
// BaseService delegates every interface method through to next. We embed a
// recorder per method so we can assert delegation without depending on
// concrete behaviour from a sibling test file.
type recordingNext struct {
	getCalled         bool
	getByEmail        bool
	provisionCalled   bool
	updateCalled      bool
	deleteCalled      bool
	getConfigCalled   bool
	patchConfigCalled bool

	getReturn        *usermodel.User
	getErr           error
	getByEmailReturn *usermodel.User
	getByEmailErr    error
	provisionRet     *usermodel.User
	provisionErr     error
	updateErr        error
	deleteErr        error
}

func (r *recordingNext) Get(_ context.Context, _ string) (*usermodel.User, error) {
	r.getCalled = true
	return r.getReturn, r.getErr
}
func (r *recordingNext) GetByEmail(_ context.Context, _ string) (*usermodel.User, error) {
	r.getByEmail = true
	return r.getByEmailReturn, r.getByEmailErr
}
func (r *recordingNext) Provision(_ context.Context, _, _ string) (*usermodel.User, error) {
	r.provisionCalled = true
	return r.provisionRet, r.provisionErr
}
func (r *recordingNext) UpdateProfile(_ context.Context, _, _ string) error {
	r.updateCalled = true
	return r.updateErr
}
func (r *recordingNext) Delete(_ context.Context, _ string) error {
	r.deleteCalled = true
	return r.deleteErr
}
func (r *recordingNext) GetConfig(_ context.Context, _ string) (*usermodel.UserConfig, error) {
	r.getConfigCalled = true
	return &usermodel.UserConfig{}, nil
}
func (r *recordingNext) PatchConfig(_ context.Context, _ string, partial usermodel.UserConfig) (*usermodel.UserConfig, error) {
	r.patchConfigCalled = true
	return &partial, nil
}

// baseHarness is a do-nothing decorator that embeds BaseService. Calling its
// methods exercises the BaseService delegation path — this proves that
// methods not overridden by a real layer still reach the underlying chain
// without being silently dropped by the embed pattern.
type baseHarness struct {
	usersvc.BaseService
}

func newBaseHarness(next usersvc.Service) *baseHarness {
	return &baseHarness{BaseService: usersvc.NewBaseService(next)}
}

func TestBaseService_DelegatesEveryMethod(t *testing.T) {
	t.Parallel()
	wantUser := &usermodel.User{ID: "u1"}
	wantErr := errors.New("boom")
	next := &recordingNext{
		getReturn:        wantUser,
		getByEmailReturn: wantUser,
		provisionRet:     wantUser,
		updateErr:        wantErr,
		deleteErr:        wantErr,
	}
	b := newBaseHarness(next)

	// Get
	if got, err := b.Get(context.Background(), "u1"); err != nil || got != wantUser {
		t.Errorf("Get returned (%v, %v), want (user, nil)", got, err)
	}
	if !next.getCalled {
		t.Error("next.Get not invoked")
	}

	// GetByEmail
	if got, err := b.GetByEmail(context.Background(), "a@b.com"); err != nil || got != wantUser {
		t.Errorf("GetByEmail returned (%v, %v), want (user, nil)", got, err)
	}
	if !next.getByEmail {
		t.Error("next.GetByEmail not invoked")
	}

	// Provision
	if got, err := b.Provision(context.Background(), "u1", "a@b.com"); err != nil || got != wantUser {
		t.Errorf("Provision returned (%v, %v), want (user, nil)", got, err)
	}
	if !next.provisionCalled {
		t.Error("next.Provision not invoked")
	}

	// UpdateProfile (propagates the error)
	if err := b.UpdateProfile(context.Background(), "u1", "Alice"); !errors.Is(err, wantErr) {
		t.Errorf("UpdateProfile error = %v, want %v", err, wantErr)
	}
	if !next.updateCalled {
		t.Error("next.UpdateProfile not invoked")
	}

	// Delete (propagates the error)
	if err := b.Delete(context.Background(), "u1"); !errors.Is(err, wantErr) {
		t.Errorf("Delete error = %v, want %v", err, wantErr)
	}
	if !next.deleteCalled {
		t.Error("next.Delete not invoked")
	}

	// GetConfig
	if _, err := b.GetConfig(context.Background(), "u1"); err != nil {
		t.Errorf("GetConfig error = %v", err)
	}
	if !next.getConfigCalled {
		t.Error("next.GetConfig not invoked")
	}

	// PatchConfig
	if _, err := b.PatchConfig(context.Background(), "u1", usermodel.UserConfig{}); err != nil {
		t.Errorf("PatchConfig error = %v", err)
	}
	if !next.patchConfigCalled {
		t.Error("next.PatchConfig not invoked")
	}
}

// NewBaseService must panic when next is nil — guards against a misconfigured
// wire setup ever instantiating a Service with a nil downstream.
func TestBaseService_NilNextPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil next")
		}
	}()
	_ = usersvc.NewBaseService(nil)
}

// NewBaseAdminService likewise panics on nil next.
func TestBaseAdminService_NilNextPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil next")
		}
	}()
	_ = usersvc.NewBaseAdminService(nil)
}

// adminBaseHarness embeds BaseAdminService directly so we can exercise the
// pass-through methods that the admin impl + validation layers do NOT
// override (List, GetWithRoles) and the ones they do (SetRoles, SetStatus,
// AdminDelete). With no overrides at all, every method must forward to
// next — proving BaseAdminService is wired correctly.
type adminBaseHarness struct {
	usersvc.BaseAdminService
}

func newAdminBaseHarness(next usersvc.AdminService) *adminBaseHarness {
	return &adminBaseHarness{BaseAdminService: usersvc.NewBaseAdminService(next)}
}

func TestBaseAdminService_DelegatesEveryMethod(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{
		listResult: usersvc.AdminListResult{Total: 11},
		getResult:  &usersvc.UserWithRoles{Roles: []string{"user"}},
	}
	b := newAdminBaseHarness(repo)
	ctx := context.Background()

	if _, err := b.List(ctx, usersvc.AdminUserFilter{}, 1, 10); err != nil {
		t.Errorf("List: %v", err)
	}
	if !repo.listCalled {
		t.Error("next.List not invoked")
	}
	if _, err := b.GetWithRoles(ctx, "u1"); err != nil {
		t.Errorf("GetWithRoles: %v", err)
	}
	if !repo.getCalled {
		t.Error("next.GetWithRoles not invoked")
	}
	if err := b.SetRoles(ctx, "u1", []string{"admin"}, "actor"); err != nil {
		t.Errorf("SetRoles: %v", err)
	}
	if !repo.setRolesCalled {
		t.Error("next.SetRoles not invoked")
	}
	if err := b.SetStatus(ctx, "u1", "active"); err != nil {
		t.Errorf("SetStatus: %v", err)
	}
	if !repo.setStatusCalled {
		t.Error("next.SetStatus not invoked")
	}
	if err := b.AdminDelete(ctx, "u1"); err != nil {
		t.Errorf("AdminDelete: %v", err)
	}
	if !repo.deleteCalled {
		t.Error("next.AdminDelete not invoked")
	}
}

// NewRestService panics on nil next — same guard. Exercises the
// constructor that drives the /api/me HTTP layer.
func TestRestService_NilNextPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil next")
		}
	}()
	_ = usersvc.NewRestService(nil)
}
