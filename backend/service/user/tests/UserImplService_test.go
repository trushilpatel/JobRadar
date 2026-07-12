package tests

import (
	"context"
	"errors"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/platform/clerkadmin"
	userdto "github.com/trushilpatel/jobradar/service/user/dto"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
)

// fakeUserRepo is the minimal Service implementation tests need to drive
// the impl layer.
type fakeUserRepo struct {
	getCalled       bool
	getReturnUser   *usermodel.User
	getReturnErr    error
	deleteCalled    bool
	deleteReturnErr error
	updateCalled    bool
	updateName      string
	updateReturnErr error
	provisionCalled bool
	provisionUser   *usermodel.User
	provisionErr    error
}

func (f *fakeUserRepo) Get(_ context.Context, _ string) (*usermodel.User, error) {
	f.getCalled = true
	return f.getReturnUser, f.getReturnErr
}
func (f *fakeUserRepo) GetByEmail(_ context.Context, _ string) (*usermodel.User, error) {
	return nil, nil
}
func (f *fakeUserRepo) Provision(_ context.Context, _, _ string) (*usermodel.User, error) {
	f.provisionCalled = true
	return f.provisionUser, f.provisionErr
}
func (f *fakeUserRepo) UpdateProfile(_ context.Context, _, name string) error {
	f.updateCalled = true
	f.updateName = name
	return f.updateReturnErr
}
func (f *fakeUserRepo) Delete(_ context.Context, _ string) error {
	f.deleteCalled = true
	return f.deleteReturnErr
}
func (f *fakeUserRepo) GetConfig(_ context.Context, _ string) (*userdto.UserConfig, error) {
	return &userdto.UserConfig{}, nil
}
func (f *fakeUserRepo) PatchConfig(_ context.Context, _ string, partial userdto.UserConfig) (*userdto.UserConfig, error) {
	return &partial, nil
}

// fakeClerk records DeleteUser calls and can be primed to return an error.
type fakeClerk struct {
	called bool
	userID string
	err    error
}

func (f *fakeClerk) DeleteUser(_ context.Context, id string) error {
	f.called = true
	f.userID = id
	return f.err
}

func TestImplService_DeleteCallsClerkFirst(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{}
	sb := &fakeClerk{}
	s := usersvc.NewImplService(repo, sb)

	if err := s.Delete(context.Background(), "user-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !sb.called {
		t.Fatal("expected clerk DeleteUser to be called")
	}
	if !repo.deleteCalled {
		t.Fatal("expected repo Delete to be called")
	}
	if sb.userID != "user-1" {
		t.Errorf("clerk userID = %q, want %q", sb.userID, "user-1")
	}
}

func TestImplService_DeleteAbortsOnClerkFailure(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{}
	sb := &fakeClerk{err: errors.New("clerk down")}
	s := usersvc.NewImplService(repo, sb)

	err := s.Delete(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected error when clerk fails")
	}
	if repo.deleteCalled {
		t.Fatal("repo Delete must not run when clerk delete fails")
	}
}

// Same tolerance as the admin delete path: if the Clerk account is already
// gone (e.g. a prior delete attempt's Clerk call succeeded but the DB delete
// step afterward failed), clerkadmin.ErrUserNotFound must not abort the
// chain — otherwise self-service account deletion is permanently broken for
// that user (every retry 500s forever with no way to actually delete).
func TestImplService_DeleteProceedsWhenClerkUserAlreadyGone(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{}
	sb := &fakeClerk{err: clerkadmin.ErrUserNotFound}
	s := usersvc.NewImplService(repo, sb)

	if err := s.Delete(context.Background(), "user-1"); err != nil {
		t.Fatalf("Delete must succeed when the Clerk user is already gone: %v", err)
	}
	if !repo.deleteCalled {
		t.Fatal("expected repo Delete to still run when Clerk returns ErrUserNotFound")
	}
}

func TestImplService_DeleteRejectsEmptyID(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{}
	s := usersvc.NewImplService(repo, &fakeClerk{})
	err := s.Delete(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty id")
	}
	var sErr *domainerrs.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "BAD_REQUEST" {
		t.Errorf("expected BAD_REQUEST, got %v", err)
	}
}

func TestImplService_GetDelegates(t *testing.T) {
	t.Parallel()
	want := &usermodel.User{ID: "u1", Email: "a@b.com"}
	repo := &fakeUserRepo{getReturnUser: want}
	s := usersvc.NewImplService(repo, nil)

	got, err := s.Get(context.Background(), "u1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("Get returned wrong user")
	}
	if !repo.getCalled {
		t.Fatal("expected repo Get to be called")
	}
}
