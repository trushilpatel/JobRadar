package services_test

import (
	"database/sql"
	"sync"
	"testing"

	sqlite3 "github.com/mattn/go-sqlite3"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// jobTestDriverName names a sqlite3 driver registered exactly once per test
// binary that exposes GREATEST/LEAST as scalar functions. user_job_counters
// uses GREATEST(col + ?, 0) which Postgres provides natively; SQLite has no
// builtin so we register equivalents here. Avoids splitting the production
// SQL by dialect.
const jobTestDriverName = "sqlite3_jobradar_test"

var registerJobTestDriver sync.Once

func ensureJobTestDriver() {
	registerJobTestDriver.Do(func() {
		sql.Register(jobTestDriverName, &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				greatest := func(a, b int64) int64 {
					if a > b {
						return a
					}
					return b
				}
				least := func(a, b int64) int64 {
					if a < b {
						return a
					}
					return b
				}
				if err := conn.RegisterFunc("greatest", greatest, true); err != nil {
					return err
				}
				return conn.RegisterFunc("least", least, true)
			},
		})
	})
}

// newJobTestDB returns an in-memory sqlite DB with the job-domain tables
// migrated from the GORM models. The Postgres-specific cases (NOW() -
// INTERVAL arithmetic, ILIKE, jsonb columns) are exercised in the DryRun
// sqlprobe tests; this helper covers the simple CRUD path that translates
// cleanly to sqlite, plus a registered GREATEST/LEAST shim so the counter
// UPDATE SQL works without a dialect split.
//
// Tables created:
//   - companies (parent for FK relationship)
//   - jobs (main table, FK to companies)
//   - user_job_state
//   - user_job_counters
//   - job_pipeline_ledger (model: JobSeenLedger)
//   - job_analytics       (model: JobSnapshot)
//   - job_tech_tags
func newJobTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	ensureJobTestDriver()
	db, err := gorm.Open(
		sqlite.New(sqlite.Config{DriverName: jobTestDriverName, DSN: ":memory:"}),
		&gorm.Config{Logger: gormlogger.Discard},
	)
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
	// Mirror migration 071: jobs.job_post_id is uniquely indexed in production.
	// InsertJob relies on ON CONFLICT (job_post_id) DO NOTHING — sqlite only
	// honours that conflict target if a matching unique index exists.
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_jobs_job_post_id ON jobs(job_post_id)`).Error; err != nil {
		t.Fatalf("create unique index: %v", err)
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
