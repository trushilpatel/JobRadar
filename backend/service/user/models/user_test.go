package usermodel

import "testing"

// Verifies status constants line up with the CHECK constraint values
// declared in migration 043 (users.status IN ('active', 'suspended')).
func TestStatusConstants(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"StatusActive":    StatusActive,
		"StatusSuspended": StatusSuspended,
	}
	want := map[string]string{
		"StatusActive":    "active",
		"StatusSuspended": "suspended",
	}
	for name, got := range cases {
		if got != want[name] {
			t.Errorf("%s = %q, want %q", name, got, want[name])
		}
	}
}

func TestUserTableName(t *testing.T) {
	t.Parallel()

	if got := (User{}).TableName(); got != "users" {
		t.Errorf("TableName() = %q, want %q", got, "users")
	}
}
