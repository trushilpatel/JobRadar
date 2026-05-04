package tests

import (
	"context"
	"testing"

	rolemodel "github.com/trushilpatel/jobradar/service/role/models"
	rolesvc "github.com/trushilpatel/jobradar/service/role/services"
)

type recordingRepo struct {
	listForCalled bool
	grantCalled   bool
	listForResult []string
}

func (r *recordingRepo) ListForUser(_ context.Context, _ string) ([]string, error) {
	r.listForCalled = true
	return r.listForResult, nil
}
func (r *recordingRepo) Grant(_ context.Context, _, _ string, _ *string) error {
	r.grantCalled = true
	return nil
}
func (r *recordingRepo) Revoke(_ context.Context, _, _ string) error { return nil }
func (r *recordingRepo) ListAll(_ context.Context) ([]rolemodel.Role, error) {
	return nil, nil
}

func TestRoleImpl_DelegatesEverything(t *testing.T) {
	t.Parallel()
	repo := &recordingRepo{listForResult: []string{"user", "admin"}}
	s := rolesvc.NewImplService(repo)

	roles, err := s.ListForUser(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 2 {
		t.Errorf("len roles = %d, want 2", len(roles))
	}
	if !repo.listForCalled {
		t.Fatal("expected delegation to repo.ListForUser")
	}
	if err := s.Grant(context.Background(), "u1", "admin", nil); err != nil {
		t.Fatal(err)
	}
	if !repo.grantCalled {
		t.Fatal("expected delegation to repo.Grant")
	}
}
