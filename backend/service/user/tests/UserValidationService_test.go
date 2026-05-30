package tests

import (
	"context"
	"errors"
	"strings"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	userdto "github.com/trushilpatel/jobradar/service/user/dto"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
)

type passthroughRepo struct {
	updateCalled bool
	updateName   string
	provisioned  bool
	provEmail    string
}

func (p *passthroughRepo) Get(_ context.Context, _ string) (*usermodel.User, error) {
	return nil, nil
}
func (p *passthroughRepo) GetByEmail(_ context.Context, _ string) (*usermodel.User, error) {
	return nil, nil
}
func (p *passthroughRepo) Provision(_ context.Context, _, email string) (*usermodel.User, error) {
	p.provisioned = true
	p.provEmail = email
	return &usermodel.User{ID: "u1", Email: email}, nil
}
func (p *passthroughRepo) UpdateProfile(_ context.Context, _, name string) error {
	p.updateCalled = true
	p.updateName = name
	return nil
}
func (p *passthroughRepo) Delete(_ context.Context, _ string) error { return nil }
func (p *passthroughRepo) GetConfig(_ context.Context, _ string) (*userdto.UserConfig, error) {
	return &userdto.UserConfig{}, nil
}
func (p *passthroughRepo) PatchConfig(_ context.Context, _ string, partial userdto.UserConfig) (*userdto.UserConfig, error) {
	return &partial, nil
}

func TestValidation_UpdateProfile_RejectsEmpty(t *testing.T) {
	t.Parallel()
	repo := &passthroughRepo{}
	s := usersvc.NewValidationService(repo)
	err := s.UpdateProfile(context.Background(), "u1", "   ")
	if err == nil {
		t.Fatal("expected validation error")
	}
	var verrs domainerrs.ValidationErrors
	if !errors.As(err, &verrs) {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}
	if repo.updateCalled {
		t.Fatal("repo.UpdateProfile must not run on validation failure")
	}
}

func TestValidation_UpdateProfile_RejectsTooLong(t *testing.T) {
	t.Parallel()
	repo := &passthroughRepo{}
	s := usersvc.NewValidationService(repo)
	long := strings.Repeat("a", 121)
	err := s.UpdateProfile(context.Background(), "u1", long)
	if err == nil {
		t.Fatal("expected too-long error")
	}
}

func TestValidation_UpdateProfile_TrimsAndAccepts(t *testing.T) {
	t.Parallel()
	repo := &passthroughRepo{}
	s := usersvc.NewValidationService(repo)
	if err := s.UpdateProfile(context.Background(), "u1", "  Alice  "); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	if !repo.updateCalled {
		t.Fatal("repo.UpdateProfile not invoked")
	}
	if repo.updateName != "Alice" {
		t.Errorf("display name not trimmed; got %q", repo.updateName)
	}
}

func TestValidation_Provision_RejectsBadEmail(t *testing.T) {
	t.Parallel()
	repo := &passthroughRepo{}
	s := usersvc.NewValidationService(repo)
	cases := []string{"", "no-at", "no-domain@", "@no-local", "missing-tld@x"}
	for _, c := range cases {
		c := c
		t.Run(c, func(t *testing.T) {
			t.Parallel()
			_, err := s.Provision(context.Background(), "u1", c)
			if err == nil {
				t.Errorf("expected error for %q", c)
			}
		})
	}
}

func TestValidation_Provision_AcceptsGood(t *testing.T) {
	t.Parallel()
	repo := &passthroughRepo{}
	s := usersvc.NewValidationService(repo)
	if _, err := s.Provision(context.Background(), "u1", "alice@example.com"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !repo.provisioned {
		t.Fatal("repo.Provision not invoked")
	}
}
