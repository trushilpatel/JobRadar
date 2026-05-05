// Package testutil hosts cross-domain test helpers shared by the per-domain
// tests/ folders. It must not import any concrete service slice to avoid
// pulling implementation code into test binaries unnecessarily.
package testutil

import "testing"

// AssertUserScoped checks that fn returns a non-nil error when called with
// an empty userID. Phase 2 uses this to lock down every user-scoped repo
// method — a missing userID must fail loudly so a forgotten ctx scoping
// never silently broadens a query to all users.
//
// Example usage (Phase 2):
//
//	testutil.AssertUserScoped(t, "JobRepo.List", func(uid string) error {
//	    _, err := repo.List(ctx, uid, ListInput{})
//	    return err
//	})
func AssertUserScoped(t *testing.T, name string, fn func(userID string) error) {
	t.Helper()
	if fn == nil {
		t.Fatalf("AssertUserScoped(%s): fn must not be nil", name)
	}
	if err := fn(""); err == nil {
		t.Errorf("AssertUserScoped(%s): expected error for empty userID, got nil", name)
	}
}
