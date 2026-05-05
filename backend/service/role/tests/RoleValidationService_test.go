package tests

import (
	"context"
	"errors"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	rolemodel "github.com/trushilpatel/jobradar/service/role/models"
	rolesvc "github.com/trushilpatel/jobradar/service/role/services"
)

type fakeRoleRepo struct {
	grantCalled  bool
	revokeCalled bool
}

func (f *fakeRoleRepo) ListForUser(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (f *fakeRoleRepo) Grant(_ context.Context, _, _ string, _ *string) error {
	f.grantCalled = true
	return nil
}
func (f *fakeRoleRepo) Revoke(_ context.Context, _, _ string) error {
	f.revokeCalled = true
	return nil
}
func (f *fakeRoleRepo) ListAll(_ context.Context) ([]rolemodel.Role, error) {
	return nil, nil
}

func TestRoleValidation_RejectsUnknownRole(t *testing.T) {
	t.Parallel()
	repo := &fakeRoleRepo{}
	s := rolesvc.NewValidationService(repo)
	err := s.Grant(context.Background(), "u1", "moderator", nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	var verrs domainerrs.ValidationErrors
	if !errors.As(err, &verrs) {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}
	if repo.grantCalled {
		t.Fatal("repo.Grant must not be called for invalid role")
	}
}

func TestRoleValidation_AcceptsKnownRoles(t *testing.T) {
	t.Parallel()
	repo := &fakeRoleRepo{}
	s := rolesvc.NewValidationService(repo)
	for _, role := range []string{rolemodel.RoleUser, rolemodel.RoleAdmin} {
		role := role
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			if err := s.Grant(context.Background(), "u1", role, nil); err != nil {
				t.Errorf("expected success for %q, got %v", role, err)
			}
		})
	}
}

func TestRoleValidation_RevokeRejectsUnknownRole(t *testing.T) {
	t.Parallel()
	repo := &fakeRoleRepo{}
	s := rolesvc.NewValidationService(repo)
	if err := s.Revoke(context.Background(), "u1", "moderator"); err == nil {
		t.Fatal("expected error")
	}
	if repo.revokeCalled {
		t.Fatal("repo.Revoke must not be called for invalid role")
	}
}
