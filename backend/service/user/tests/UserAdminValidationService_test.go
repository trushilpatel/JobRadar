package tests

import (
	"context"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
)

// adminValidationCases drives every guard on the admin validation layer.
// Each subtest asserts that the validation layer rejects bad input with
// BAD_REQUEST *before* the request reaches next.

func TestAdminValidation_GetWithRoles_EmptyID(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	v := usersvc.NewAdminValidationService(repo)
	_, err := v.GetWithRoles(context.Background(), "")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrBadRequest.Code {
		t.Errorf("code = %q, want BAD_REQUEST", se.Code)
	}
	if repo.getCalled {
		t.Error("next.GetWithRoles must not run on validation failure")
	}
}

func TestAdminValidation_GetWithRoles_Passthrough(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{getResult: &usersvc.UserWithRoles{}}
	v := usersvc.NewAdminValidationService(repo)
	if _, err := v.GetWithRoles(context.Background(), "u1"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !repo.getCalled {
		t.Error("next.GetWithRoles not invoked")
	}
}

func TestAdminValidation_SetRoles_GuardsBothIDs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		id        string
		grantedBy string
	}{
		{"empty id", "", "actor"},
		{"empty granted_by", "u1", ""},
		{"both empty", "", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &fakeAdminRepo{}
			v := usersvc.NewAdminValidationService(repo)
			err := v.SetRoles(context.Background(), tc.id, []string{"admin"}, tc.grantedBy)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrBadRequest.Code {
				t.Errorf("code = %q, want BAD_REQUEST", se.Code)
			}
			if repo.setRolesCalled {
				t.Error("next.SetRoles must not run on validation failure")
			}
		})
	}
}

func TestAdminValidation_SetRoles_Passthrough(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	v := usersvc.NewAdminValidationService(repo)
	if err := v.SetRoles(context.Background(), "u1", []string{"admin"}, "actor"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !repo.setRolesCalled {
		t.Error("next.SetRoles not invoked")
	}
}

func TestAdminValidation_SetStatus_EmptyID(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	v := usersvc.NewAdminValidationService(repo)
	err := v.SetStatus(context.Background(), "", usermodel.StatusActive)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrBadRequest.Code {
		t.Errorf("code = %q, want BAD_REQUEST", se.Code)
	}
	if repo.setStatusCalled {
		t.Error("next.SetStatus must not run on empty id")
	}
}

// The validation layer must reject any status outside the (active, suspended)
// pair so the DB CHECK constraint is never the line of defence.
func TestAdminValidation_SetStatus_RejectsUnknownValue(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	v := usersvc.NewAdminValidationService(repo)
	for _, bad := range []string{"", "ACTIVE", "deleted", " active ", "anything"} {
		bad := bad
		t.Run(bad, func(t *testing.T) {
			t.Parallel()
			err := v.SetStatus(context.Background(), "u1", bad)
			if err == nil {
				t.Errorf("status %q passed validation", bad)
				return
			}
			if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrBadRequest.Code {
				t.Errorf("status %q: code = %q, want BAD_REQUEST", bad, se.Code)
			}
		})
	}
}

func TestAdminValidation_SetStatus_AcceptsActiveAndSuspended(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	v := usersvc.NewAdminValidationService(repo)
	for _, ok := range []string{usermodel.StatusActive, usermodel.StatusSuspended} {
		if err := v.SetStatus(context.Background(), "u1", ok); err != nil {
			t.Errorf("status %q rejected: %v", ok, err)
		}
	}
	if !repo.setStatusCalled {
		t.Error("next.SetStatus not invoked")
	}
}

func TestAdminValidation_AdminDelete_EmptyID(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	v := usersvc.NewAdminValidationService(repo)
	err := v.AdminDelete(context.Background(), "")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrBadRequest.Code {
		t.Errorf("code = %q, want BAD_REQUEST", se.Code)
	}
	if repo.deleteCalled {
		t.Error("next.AdminDelete must not run on empty id")
	}
}

func TestAdminValidation_AdminDelete_Passthrough(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	v := usersvc.NewAdminValidationService(repo)
	if err := v.AdminDelete(context.Background(), "u1"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !repo.deleteCalled {
		t.Error("next.AdminDelete not invoked")
	}
}

// List is not overridden — it should pass straight through.
func TestAdminValidation_ListDelegates(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{listResult: usersvc.AdminListResult{Total: 7}}
	v := usersvc.NewAdminValidationService(repo)
	res, err := v.List(context.Background(), usersvc.AdminUserFilter{}, 1, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if res.Total != 7 {
		t.Errorf("Total = %d, want 7 (delegation)", res.Total)
	}
	if !repo.listCalled {
		t.Error("next.List not invoked")
	}
}
