package services_test

import (
	"testing"

	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newJobTestDB returns an in-memory sqlite DB with the job-domain tables
// migrated from the GORM models. The Postgres-specific cases (NOW() -
// INTERVAL arithmetic, ILIKE, jsonb columns) are exercised in the DryRun
// sqlprobe tests; this helper covers the simple CRUD path that translates
// cleanly to sqlite.
//
// Tables created:
//   - companies (parent for FK relationship)
//   - jobs (main table, FK to companies)
//   - user_job_state
//   - job_pipeline_ledger (model: JobSeenLedger)
//   - job_analytics       (model: JobSnapshot)
//   - job_tech_tags
func newJobTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Enable FK enforcement so cascades behave like Postgres.
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable fks: %v", err)
	}
	// Companies is the parent table — migrate first so FKs resolve.
	if err := db.AutoMigrate(
		&companymodel.Company{},
		&jobmodel.Job{},
		&jobmodel.UserJobState{},
		&jobmodel.UserJobCounters{},
		&jobmodel.JobSeenLedger{},
		&jobmodel.JobSnapshot{},
		&jobmodel.JobTechTag{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// seedCompany inserts a minimal company row so FK-bearing jobs/ledger rows
// can resolve their parent.
func seedCompany(t *testing.T, db *gorm.DB, id, name string) {
	t.Helper()
	c := companymodel.Company{ID: id, Name: name, Slug: id}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("seed company %s: %v", id, err)
	}
}
