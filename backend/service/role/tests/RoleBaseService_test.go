package tests

import (
	"context"
	"errors"
	"testing"

	rolemodel "github.com/trushilpatel/jobradar/service/role/models"
	rolesvc "github.com/trushilpatel/jobradar/service/role/services"
)

// recordingDelegateRepo captures which BaseService method delegated and
// returns deterministic values so each delegation can be asserted
// independently. Each method also returns an injectable error so the
// delegation path is exercised when the next layer fails.
type recordingDelegateRepo struct {
	listForCalled  bool
	listForUserID  string
	listForResult  []string
	listForErr     error
	grantCalled    bool
	grantUserID    string
	grantRoleID    string
	grantGrantedBy *string
	grantErr       error
	revokeCalled   bool
	revokeUserID   string
	revokeRoleID   string
	revokeErr      error
	listAllCalled  bool
	listAllResult  []rolemodel.Role
	listAllErr     error
}

func (r *recordingDelegateRepo) ListForUser(_ context.Context, userID string) ([]string, error) {
	r.listForCalled = true
	r.listForUserID = userID
	return r.listForResult, r.listForErr
}
func (r *recordingDelegateRepo) Grant(_ context.Context, userID, roleID string, grantedBy *string) error {
	r.grantCalled = true
	r.grantUserID = userID
	r.grantRoleID = roleID
	r.grantGrantedBy = grantedBy
	return r.grantErr
}
func (r *recordingDelegateRepo) Revoke(_ context.Context, userID, roleID string) error {
	r.revokeCalled = true
	r.revokeUserID = userID
	r.revokeRoleID = roleID
	return r.revokeErr
}
func (r *recordingDelegateRepo) ListAll(_ context.Context) ([]rolemodel.Role, error) {
	r.listAllCalled = true
	return r.listAllResult, r.listAllErr
}

// NewBaseService must panic on a nil next — a misconfigured chain is a
// programmer error, not a runtime condition the system should silently
// handle. The Impl/Validation constructors both rely on this guarantee.
func TestRoleBaseService_NewBaseService_NilPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil next")
		}
	}()
	rolesvc.NewBaseService(nil)
}

// Exercise every BaseService delegation method through both ImplService and
// ValidationService. Coverage of the BaseService methods only counts when
// the layer above them does *not* override the method — Impl overrides
// nothing, so its instance walks through every BaseService method.
func TestRoleBaseService_ImplDelegatesAllMethods(t *testing.T) {
	t.Parallel()

	want := []rolemodel.Role{{ID: rolemodel.RoleUser}}
	repo := &recordingDelegateRepo{
		listForResult: []string{rolemodel.RoleAdmin},
		listAllResult: want,
	}
	s := rolesvc.NewImplService(repo)
	ctx := context.Background()

	roles, err := s.ListForUser(ctx, "u-list")
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if !repo.listForCalled || repo.listForUserID != "u-list" {
		t.Errorf("ListForUser delegation: called=%v userID=%q", repo.listForCalled, repo.listForUserID)
	}
	if len(roles) != 1 || roles[0] != rolemodel.RoleAdmin {
		t.Errorf("roles = %v, want [admin]", roles)
	}

	gb := "granted-by-1"
	if err := s.Grant(ctx, "u-grant", rolemodel.RoleAdmin, &gb); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if !repo.grantCalled || repo.grantUserID != "u-grant" || repo.grantRoleID != rolemodel.RoleAdmin {
		t.Errorf("Grant delegation: called=%v userID=%q roleID=%q",
			repo.grantCalled, repo.grantUserID, repo.grantRoleID)
	}
	if repo.grantGrantedBy == nil || *repo.grantGrantedBy != gb {
		t.Errorf("grantedBy not forwarded: %v", repo.grantGrantedBy)
	}

	if err := s.Revoke(ctx, "u-revoke", rolemodel.RoleUser); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if !repo.revokeCalled || repo.revokeUserID != "u-revoke" || repo.revokeRoleID != rolemodel.RoleUser {
		t.Errorf("Revoke delegation: called=%v userID=%q roleID=%q",
			repo.revokeCalled, repo.revokeUserID, repo.revokeRoleID)
	}

	rows, err := s.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if !repo.listAllCalled {
		t.Fatal("ListAll not delegated")
	}
	if len(rows) != 1 || rows[0].ID != want[0].ID {
		t.Errorf("rows = %v, want %v", rows, want)
	}
}

// Errors from `next` must be passed through unchanged — BaseService never
// wraps or rewrites them. A handler downstream relies on the same
// *ServiceError reaching it intact.
func TestRoleBaseService_PropagatesNextError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("downstream boom")
	repo := &recordingDelegateRepo{
		listForErr: wantErr,
		grantErr:   wantErr,
		revokeErr:  wantErr,
		listAllErr: wantErr,
	}
	s := rolesvc.NewImplService(repo)
	ctx := context.Background()

	if _, err := s.ListForUser(ctx, "u"); !errors.Is(err, wantErr) {
		t.Errorf("ListForUser err = %v, want %v", err, wantErr)
	}
	if err := s.Grant(ctx, "u", rolemodel.RoleUser, nil); !errors.Is(err, wantErr) {
		t.Errorf("Grant err = %v, want %v", err, wantErr)
	}
	if err := s.Revoke(ctx, "u", rolemodel.RoleUser); !errors.Is(err, wantErr) {
		t.Errorf("Revoke err = %v, want %v", err, wantErr)
	}
	if _, err := s.ListAll(ctx); !errors.Is(err, wantErr) {
		t.Errorf("ListAll err = %v, want %v", err, wantErr)
	}
}

// Validation only overrides Grant and Revoke — ListForUser and ListAll must
// pass straight through the embedded BaseService delegation. This pins the
// guarantee that the validation layer never silently blocks read calls.
func TestRoleValidation_NonGrantMethodsPassThrough(t *testing.T) {
	t.Parallel()
	repo := &recordingDelegateRepo{
		listForResult: []string{rolemodel.RoleUser, rolemodel.RoleAdmin},
		listAllResult: []rolemodel.Role{{ID: rolemodel.RoleUser}, {ID: rolemodel.RoleAdmin}},
	}
	s := rolesvc.NewValidationService(repo)
	ctx := context.Background()

	got, err := s.ListForUser(ctx, "u1")
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("ids = %v, want 2", got)
	}
	if !repo.listForCalled {
		t.Fatal("ListForUser must delegate through BaseService")
	}

	rows, err := s.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if !repo.listAllCalled {
		t.Fatal("ListAll must delegate through BaseService")
	}
	if len(rows) != 2 {
		t.Errorf("rows = %v, want 2", rows)
	}
}

// Revoke must accept every role ID the application is allowed to grant.
// The existing suite only checks rejection of unknown values; this pins
// both known values for the revoke path, completing the table.
func TestRoleValidation_RevokeAcceptsKnownRoles(t *testing.T) {
	t.Parallel()
	for _, role := range []string{rolemodel.RoleUser, rolemodel.RoleAdmin} {
		role := role
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			repo := &recordingDelegateRepo{}
			s := rolesvc.NewValidationService(repo)
			if err := s.Revoke(context.Background(), "u1", role); err != nil {
				t.Errorf("Revoke(%q) failed: %v", role, err)
			}
			if !repo.revokeCalled {
				t.Errorf("expected revoke to delegate for %q", role)
			}
		})
	}
}
