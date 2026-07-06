package tests

import (
	"testing"

	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	resumemodel "github.com/trushilpatel/jobradar/service/resume/models"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newApplyQueueTestDB returns an in-memory sqlite DB with the tables the
// apply-queue repo joins across: companies (parent for jobs' FK), jobs,
// user_job_state (all three AutoMigrated from their GORM models, mirroring
// service/job/tests/repotestdb_test.go), plus hand-written resumes and
// resume_job_attachments tables — AutoMigrate on resumemodel.Resume fails on
// sqlite because its `default:gen_random_uuid()` tag is Postgres-only syntax,
// same reason service/resume/tests/ResumeRepositoryService_test.go hand-
// writes its schema instead of AutoMigrating.
func newApplyQueueTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable fks: %v", err)
	}
	if err := db.AutoMigrate(
		&companymodel.Company{},
		&jobmodel.Job{},
		&jobmodel.UserJobState{},
		&jobmodel.JobTechTag{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	stmts := []string{
		`CREATE TABLE resumes (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			label TEXT NOT NULL,
			original_filename TEXT NOT NULL,
			storage_key TEXT NOT NULL UNIQUE,
			content_type TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			extracted_text TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE resume_job_attachments (
			user_id TEXT NOT NULL,
			job_id TEXT NOT NULL,
			resume_id TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, job_id)
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("schema setup: %v", err)
		}
	}
	return db
}

// seedCompany inserts a minimal company row so FK-bearing job rows resolve.
func seedCompany(t *testing.T, db *gorm.DB, id, name string) {
	t.Helper()
	c := companymodel.Company{ID: id, Name: name, Slug: id}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("seed company %s: %v", id, err)
	}
}

// seedJob inserts a minimal job row.
func seedJob(t *testing.T, db *gorm.DB, id, companyID, title string) {
	t.Helper()
	j := jobmodel.Job{
		ID:        id,
		JobPostID: "p-" + id,
		CompanyID: companyID,
		Title:     title,
		URL:       "https://example.com/" + id,
	}
	if err := db.Create(&j).Error; err != nil {
		t.Fatalf("seed job %s: %v", id, err)
	}
}

// seedJobWithDetails inserts a job row with location/remote/salary_range set
// explicitly — used by tests asserting QueueItem exposes these pass-through
// fields (mirroring jobmodel.Job's Location/Remote/SalaryRange) distinctly
// from the null-by-default case covered by seedJob.
func seedJobWithDetails(t *testing.T, db *gorm.DB, id, companyID, title string, location *string, remote bool, salaryRange datatypes.JSON) {
	t.Helper()
	j := jobmodel.Job{
		ID:          id,
		JobPostID:   "p-" + id,
		CompanyID:   companyID,
		Title:       title,
		URL:         "https://example.com/" + id,
		Location:    location,
		Remote:      remote,
		SalaryRange: salaryRange,
	}
	if err := db.Create(&j).Error; err != nil {
		t.Fatalf("seed job with details %s: %v", id, err)
	}
}

// seedResume inserts a minimal resume row owned by userID.
func seedResume(t *testing.T, db *gorm.DB, id, userID, label string) {
	t.Helper()
	r := resumemodel.Resume{
		ID:               id,
		UserID:           userID,
		Label:            label,
		OriginalFilename: label + ".pdf",
		StorageKey:       "resumes/" + userID + "/" + id,
		ContentType:      "application/pdf",
		SizeBytes:        1024,
	}
	if err := db.Create(&r).Error; err != nil {
		t.Fatalf("seed resume %s: %v", id, err)
	}
}

// seedTechTags inserts job_tech_tags rows for jobID — the canonical
// (techtag.Clean-cleaned) tech set attached to apply-queue items as
// JobKeywords, as opposed to the uncleaned jobs.tech_stack JSONB column.
func seedTechTags(t *testing.T, db *gorm.DB, jobID, companyID string, tags ...string) {
	t.Helper()
	for _, tag := range tags {
		tt := jobmodel.JobTechTag{JobID: jobID, Tech: tag, CompanyID: companyID}
		if err := db.Create(&tt).Error; err != nil {
			t.Fatalf("seed tech tag (%s,%s): %v", jobID, tag, err)
		}
	}
}
