package services_test

import (
	"context"
	"testing"

	"github.com/trushilpatel/jobradar/service/job/dto"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	"github.com/trushilpatel/jobradar/service/testutil"
)

// TestUserJobStateRepository_UserScopedGuards verifies that every method
// on UserJobStateRepository rejects an empty userID with a non-nil error
// BEFORE touching the DB. Running with a nil *gorm.DB is intentional —
// the guard must fire before any db.WithContext call, so a forgotten ctx
// scoping cannot accidentally broaden a query to all users.
func TestUserJobStateRepository_UserScopedGuards(t *testing.T) {
	repo := jobsvc.NewUserJobStateRepository(nil)
	ctx := context.Background()

	testutil.AssertUserScoped(t, "Get", func(uid string) error {
		_, err := repo.Get(ctx, uid, "job-1")
		return err
	})
	testutil.AssertUserScoped(t, "GetMany", func(uid string) error {
		_, err := repo.GetMany(ctx, uid, []string{"job-1"})
		return err
	})
	testutil.AssertUserScoped(t, "Upsert", func(uid string) error {
		return repo.Upsert(ctx, uid, "job-1", dto.UpsertState{Status: "saved"})
	})
	testutil.AssertUserScoped(t, "UpsertBulkDismiss", func(uid string) error {
		return repo.UpsertBulkDismiss(ctx, uid, []string{"job-1"}, "user_dismissed", "")
	})
	testutil.AssertUserScoped(t, "Delete", func(uid string) error {
		return repo.Delete(ctx, uid, "job-1")
	})
	testutil.AssertUserScoped(t, "CountsByUser", func(uid string) error {
		_, err := repo.CountsByUser(ctx, uid)
		return err
	})
}
