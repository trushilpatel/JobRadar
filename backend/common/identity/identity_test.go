package identity_test

import (
	"context"
	"testing"

	"github.com/trushilpatel/jobradar/common/identity"
)

func TestNewContext_FromContext_Roundtrip(t *testing.T) {
	in := identity.Identity{
		UserID:      "u-1",
		Email:       "a@b.com",
		Roles:       []string{"user", "admin"},
		Status:      "active",
		DisplayName: "Alice",
	}
	ctx := identity.NewContext(context.Background(), in)

	got, ok := identity.FromContext(ctx)
	if !ok {
		t.Fatal("FromContext: want ok=true, got false")
	}
	if got.UserID != in.UserID || got.Email != in.Email || got.Status != in.Status || got.DisplayName != in.DisplayName {
		t.Fatalf("FromContext: want %+v, got %+v", in, got)
	}
	if len(got.Roles) != len(in.Roles) || got.Roles[0] != "user" || got.Roles[1] != "admin" {
		t.Fatalf("FromContext: roles mismatch: want %v, got %v", in.Roles, got.Roles)
	}
}

func TestFromContext_NotPresent(t *testing.T) {
	if _, ok := identity.FromContext(context.Background()); ok {
		t.Fatal("FromContext on bare ctx: want ok=false, got true")
	}
	if _, ok := identity.FromContext(nil); ok { //nolint:staticcheck // explicit nil-ctx guard
		t.Fatal("FromContext(nil): want ok=false, got true")
	}
}

func TestMustFromContext_PanicsWhenMissing(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustFromContext: expected panic, got none")
		}
	}()
	_ = identity.MustFromContext(context.Background())
}

func TestMustFromContext_ReturnsValue(t *testing.T) {
	in := identity.Identity{UserID: "u-2"}
	ctx := identity.NewContext(context.Background(), in)
	got := identity.MustFromContext(ctx)
	if got.UserID != "u-2" {
		t.Fatalf("MustFromContext: want u-2, got %q", got.UserID)
	}
}

func TestUserIDFrom(t *testing.T) {
	if got := identity.UserIDFrom(context.Background()); got != "" {
		t.Fatalf("UserIDFrom(empty ctx): want \"\", got %q", got)
	}
	ctx := identity.NewContext(context.Background(), identity.Identity{UserID: "u-3"})
	if got := identity.UserIDFrom(ctx); got != "u-3" {
		t.Fatalf("UserIDFrom: want u-3, got %q", got)
	}
}

func TestIsAdmin_AndHasRole(t *testing.T) {
	id := identity.Identity{Roles: []string{"user", "admin"}}
	if !id.HasRole("admin") {
		t.Fatal("HasRole(admin): want true")
	}
	if !id.HasRole("user") {
		t.Fatal("HasRole(user): want true")
	}
	if id.HasRole("moderator") {
		t.Fatal("HasRole(moderator): want false")
	}
	if !id.IsAdmin() {
		t.Fatal("IsAdmin: want true")
	}

	noAdmin := identity.Identity{Roles: []string{"user"}}
	if noAdmin.IsAdmin() {
		t.Fatal("IsAdmin(non-admin): want false")
	}
}

func TestCtx_IsAdmin_HasRole(t *testing.T) {
	ctx := identity.NewContext(context.Background(), identity.Identity{Roles: []string{"admin"}})
	if !identity.IsAdmin(ctx) {
		t.Fatal("identity.IsAdmin: want true")
	}
	if !identity.HasRole(ctx, "admin") {
		t.Fatal("identity.HasRole(admin): want true")
	}
	if identity.HasRole(ctx, "moderator") {
		t.Fatal("identity.HasRole(moderator): want false")
	}

	bare := context.Background()
	if identity.IsAdmin(bare) {
		t.Fatal("IsAdmin(bare): want false")
	}
	if identity.HasRole(bare, "admin") {
		t.Fatal("HasRole(bare): want false")
	}
}

func TestWithTestIdentity(t *testing.T) {
	ctx := identity.WithTestIdentity(context.Background(), "tester", "user", "admin")
	id, ok := identity.FromContext(ctx)
	if !ok {
		t.Fatal("WithTestIdentity: identity not found in ctx")
	}
	if id.UserID != "tester" {
		t.Fatalf("WithTestIdentity userID: want tester, got %q", id.UserID)
	}
	if !id.IsAdmin() || !id.HasRole("user") {
		t.Fatalf("WithTestIdentity roles: want admin+user, got %v", id.Roles)
	}
	if id.Status != "active" {
		t.Fatalf("WithTestIdentity status: want active, got %q", id.Status)
	}
	if got := identity.UserIDFrom(ctx); got != "tester" {
		t.Fatalf("UserIDFrom: want tester, got %q", got)
	}
}

func TestWithTestIdentity_RolesAreCopied(t *testing.T) {
	roles := []string{"user"}
	ctx := identity.WithTestIdentity(context.Background(), "u", roles...)
	roles[0] = "mutated"
	id, _ := identity.FromContext(ctx)
	if id.Roles[0] != "user" {
		t.Fatalf("WithTestIdentity must copy roles: got %q", id.Roles[0])
	}
}
