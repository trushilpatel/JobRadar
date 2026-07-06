package application

import (
	"context"
	"testing"

	"github.com/trushilpatel/jobradar/service/application/dto"
	svc "github.com/trushilpatel/jobradar/service/application/services"
	"github.com/trushilpatel/jobradar/service/testutil"
)

// TestApplicationRepository_UserScopedGuards verifies every method on
// ApplicationRepository rejects an empty userID with a non-nil error
// BEFORE touching the DB. Running with a nil *gorm.DB is intentional —
// the guard must fire before any db.WithContext call, so a forgotten
// ctx scoping cannot accidentally broaden a query to all users.
func TestApplicationRepository_UserScopedGuards(t *testing.T) {
	repo := svc.NewRepositoryService(nil)
	ctx := context.Background()

	testutil.AssertUserScoped(t, "ListByJob", func(uid string) error {
		_, err := repo.ListByJob(ctx, uid, "job-1")
		return err
	})
	testutil.AssertUserScoped(t, "ListAll", func(uid string) error {
		_, _, err := repo.ListAll(ctx, uid, 1, 20)
		return err
	})
	testutil.AssertUserScoped(t, "ListActive", func(uid string) error {
		_, err := repo.ListActive(ctx, uid)
		return err
	})
	testutil.AssertUserScoped(t, "Create", func(uid string) error {
		_, err := repo.Create(ctx, uid, "job-1", dto.CreateApplicationInput{Stage: "applied"})
		return err
	})
	testutil.AssertUserScoped(t, "UpdateStage", func(uid string) error {
		return repo.UpdateStage(ctx, uid, "1", dto.UpdateStageInput{Stage: "applied"})
	})
	testutil.AssertUserScoped(t, "UpdateNote", func(uid string) error {
		return repo.UpdateNote(ctx, uid, "1", dto.UpdateNoteInput{Note: "x"})
	})
}

// TestApplicationWriter_UserScopedGuard verifies that the
// jobsvc.ApplicationWriter contract (InsertAppliedApplication, where the
// userID is sourced from ctx) rejects a context with no identity attached.
func TestApplicationWriter_UserScopedGuard(t *testing.T) {
	writer := svc.NewApplicationWriter(nil)
	if err := writer.InsertAppliedApplication(context.Background(), "job-1"); err == nil {
		t.Error("expected error when ctx carries no identity, got nil")
	}
}
