package services_test

import (
	"context"
	"testing"

	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// TestSnapshotStore_InsertSnapshot_HappyPath inserts a row with every
// nullable field populated and reads it back.
func TestSnapshotStore_InsertSnapshot_HappyPath(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	store := jobsvc.NewSnapshotStore(db)
	expYears := 4
	input := jobdto.SnapshotInput{
		ID:                "s1",
		CompanyID:         "c1",
		Country:           "USA",
		State:             "CA",
		Remote:            true,
		Sponsorship:       false,
		RequiresCitizen:   true,
		SecurityClearance: false,
		EmployType:        jobcommon.EmployFullTime,
		ExperienceYears:   &expYears,
		TechStack:         []byte(`["Go","PostgreSQL"]`),
		RoleCategory:      "backend",
	}
	if err := store.InsertSnapshot(context.Background(), input); err != nil {
		t.Fatalf("InsertSnapshot err = %v", err)
	}

	var got jobmodel.JobSnapshot
	if err := db.Where("id = ?", "s1").First(&got).Error; err != nil {
		t.Fatalf("readback: %v", err)
	}
	if got.Country == nil || *got.Country != "USA" {
		t.Errorf("country = %v, want USA", got.Country)
	}
	if got.State == nil || *got.State != "CA" {
		t.Errorf("state = %v, want CA", got.State)
	}
	if got.EmployType == nil || *got.EmployType != jobcommon.EmployFullTime {
		t.Errorf("employ type = %v", got.EmployType)
	}
	if got.RoleCategory == nil || *got.RoleCategory != "backend" {
		t.Errorf("role category = %v", got.RoleCategory)
	}
	if !got.Remote || got.Sponsorship {
		t.Errorf("flags wrong: remote=%v sponsorship=%v", got.Remote, got.Sponsorship)
	}
}

// TestSnapshotStore_InsertSnapshot_EmptyStringsBecomeNull — empty country/
// state/employ_type/role_category fields skip the pointer assignment, so the
// stored column is NULL not "".
func TestSnapshotStore_InsertSnapshot_EmptyStringsBecomeNull(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	store := jobsvc.NewSnapshotStore(db)
	input := jobdto.SnapshotInput{
		ID:        "s2",
		CompanyID: "c1",
		Country:   "",
		State:     "",
	}
	if err := store.InsertSnapshot(context.Background(), input); err != nil {
		t.Fatalf("err = %v", err)
	}
	var got jobmodel.JobSnapshot
	if err := db.Where("id = ?", "s2").First(&got).Error; err != nil {
		t.Fatalf("readback: %v", err)
	}
	if got.Country != nil {
		t.Errorf("country = %v, want nil", *got.Country)
	}
	if got.State != nil {
		t.Errorf("state = %v, want nil", *got.State)
	}
	if got.EmployType != nil {
		t.Errorf("employ_type = %v, want nil", *got.EmployType)
	}
	if got.RoleCategory != nil {
		t.Errorf("role_category = %v, want nil", *got.RoleCategory)
	}
}

// TestSnapshotStore_UpdateSnapshotAction sets user_action + actioned_at.
func TestSnapshotStore_UpdateSnapshotAction(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	store := jobsvc.NewSnapshotStore(db)
	if err := store.InsertSnapshot(context.Background(), jobdto.SnapshotInput{ID: "s1", CompanyID: "c1"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := store.UpdateSnapshotAction(context.Background(), "s1", jobmodel.StatusSaved); err != nil {
		t.Fatalf("UpdateSnapshotAction err = %v", err)
	}
	var got jobmodel.JobSnapshot
	if err := db.Where("id = ?", "s1").First(&got).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.UserAction == nil || *got.UserAction != jobmodel.StatusSaved {
		t.Errorf("user_action = %v, want saved", got.UserAction)
	}
	if got.ActionedAt == nil || got.ActionedAt.IsZero() {
		t.Errorf("actioned_at not set: %v", got.ActionedAt)
	}
}

// TestSnapshotStore_UpdateSnapshotActionBatch_Empty short-circuits.
func TestSnapshotStore_UpdateSnapshotActionBatch_Empty(t *testing.T) {
	t.Parallel()
	store := jobsvc.NewSnapshotStore(newJobTestDB(t))
	if err := store.UpdateSnapshotActionBatch(context.Background(), nil, jobmodel.StatusDismissed); err != nil {
		t.Errorf("empty batch should no-op: %v", err)
	}
}

// TestSnapshotStore_UpdateSnapshotActionBatch updates all listed rows.
func TestSnapshotStore_UpdateSnapshotActionBatch(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	store := jobsvc.NewSnapshotStore(db)
	for _, id := range []string{"s1", "s2", "s3"} {
		if err := store.InsertSnapshot(context.Background(), jobdto.SnapshotInput{ID: id, CompanyID: "c1"}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	if err := store.UpdateSnapshotActionBatch(context.Background(), []string{"s1", "s3"}, jobmodel.StatusDismissed); err != nil {
		t.Fatalf("batch update: %v", err)
	}
	var rows []jobmodel.JobSnapshot
	if err := db.Order("id").Find(&rows).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if rows[0].UserAction == nil || *rows[0].UserAction != jobmodel.StatusDismissed {
		t.Errorf("s1.user_action = %v", rows[0].UserAction)
	}
	if rows[1].UserAction != nil {
		t.Errorf("s2.user_action = %v, want nil (not in batch)", *rows[1].UserAction)
	}
	if rows[2].UserAction == nil || *rows[2].UserAction != jobmodel.StatusDismissed {
		t.Errorf("s3.user_action = %v", rows[2].UserAction)
	}
}
