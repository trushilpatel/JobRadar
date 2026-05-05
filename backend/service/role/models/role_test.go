package rolemodel

import "testing"

// Role IDs must match the seed rows inserted by migration 043.
func TestRoleConstants(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"RoleUser":  RoleUser,
		"RoleAdmin": RoleAdmin,
	}
	want := map[string]string{
		"RoleUser":  "user",
		"RoleAdmin": "admin",
	}
	for name, got := range cases {
		if got != want[name] {
			t.Errorf("%s = %q, want %q", name, got, want[name])
		}
	}
}

func TestTableNames(t *testing.T) {
	t.Parallel()

	if got := (Role{}).TableName(); got != "roles" {
		t.Errorf("Role.TableName() = %q, want %q", got, "roles")
	}
	if got := (UserRole{}).TableName(); got != "user_roles" {
		t.Errorf("UserRole.TableName() = %q, want %q", got, "user_roles")
	}
}
