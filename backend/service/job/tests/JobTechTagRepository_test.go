package services_test

import (
	"context"
	"testing"
	"time"

	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// TestTechTagStore_InsertTechTags_Empty short-circuits and returns nil
// without touching the DB.
func TestTechTagStore_InsertTechTags_Empty(t *testing.T) {
	t.Parallel()
	store := jobsvc.NewTechTagStore(newJobTestDB(t))
	if err := store.InsertTechTags(context.Background(), "j1", "c1", nil, time.Now()); err != nil {
		t.Errorf("empty tags should no-op: %v", err)
	}
	if err := store.InsertTechTags(context.Background(), "j1", "c1", []string{}, time.Now()); err != nil {
		t.Errorf("empty slice should no-op: %v", err)
	}
}

// TestTechTagStore_InsertTechTags_HappyPath writes multiple tags and reads
// them back through GORM.
func TestTechTagStore_InsertTechTags_HappyPath(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	store := jobsvc.NewTechTagStore(db)
	now := time.Now()
	if err := store.InsertTechTags(context.Background(), "j1", "c1", []string{"Go", "PostgreSQL", "Kafka"}, now); err != nil {
		t.Fatalf("err = %v", err)
	}
	var rows []jobmodel.JobTechTag
	if err := db.Where("job_id = ?", "j1").Find(&rows).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len = %d, want 3", len(rows))
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.Tech] = true
		if r.CompanyID != "c1" {
			t.Errorf("company id = %q, want c1", r.CompanyID)
		}
		if r.TaggedAt.IsZero() {
			t.Errorf("tagged_at zero for %s", r.Tech)
		}
	}
	for _, want := range []string{"Go", "PostgreSQL", "Kafka"} {
		if !got[want] {
			t.Errorf("missing tag %q", want)
		}
	}
}

// TestTechTagStore_InsertTechTags_OnConflictDoNothing — re-inserting the
// same (job_id, tech) pair must not error and must not duplicate rows.
func TestTechTagStore_InsertTechTags_OnConflictDoNothing(t *testing.T) {
	t.Parallel()
	db := newJobTestDB(t)
	store := jobsvc.NewTechTagStore(db)
	now := time.Now()
	if err := store.InsertTechTags(context.Background(), "j1", "c1", []string{"Go", "Rust"}, now); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	// Repeat with overlap on "Go" plus new tag "Python".
	if err := store.InsertTechTags(context.Background(), "j1", "c1", []string{"Go", "Python"}, now); err != nil {
		t.Fatalf("second insert with conflict: %v", err)
	}
	var count int64
	if err := db.Model(&jobmodel.JobTechTag{}).Where("job_id = ?", "j1").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3 (Go,Rust,Python — Go dedup'd)", count)
	}
}
