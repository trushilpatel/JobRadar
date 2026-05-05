package auditmodel

import "testing"

// Action constants are referenced by the audit middleware and admin
// services — pin their string values so accidental renames fail loudly.
func TestActionConstants(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"ActionUserDelete":       ActionUserDelete,
		"ActionUserRoleGrant":    ActionUserRoleGrant,
		"ActionUserRoleRevoke":   ActionUserRoleRevoke,
		"ActionUserStatusChange": ActionUserStatusChange,
	}
	want := map[string]string{
		"ActionUserDelete":       "user.delete",
		"ActionUserRoleGrant":    "user.role.grant",
		"ActionUserRoleRevoke":   "user.role.revoke",
		"ActionUserStatusChange": "user.status.change",
	}
	for name, got := range cases {
		if got != want[name] {
			t.Errorf("%s = %q, want %q", name, got, want[name])
		}
	}
}

func TestEntryTableName(t *testing.T) {
	t.Parallel()

	if got := (Entry{}).TableName(); got != "admin_audit_log" {
		t.Errorf("TableName() = %q, want %q", got, "admin_audit_log")
	}
}
