package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	pipelinedto "github.com/trushilpatel/jobradar/service/pipeline/dto"
	pipelinesvc "github.com/trushilpatel/jobradar/service/pipeline/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newPipelineTestDB returns an in-memory SQLite DB with the five tables the
// pipeline stats query reads from. Deliberately pared down to only the
// columns the query touches — this is a stats-only slice, not the owning
// domain for any of these tables.
func newPipelineTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Discard,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	stmts := []string{
		`CREATE TABLE companies (id TEXT PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE company_scan_settings (
			company_id TEXT PRIMARY KEY,
			active     BOOLEAN NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE company_scan_queue (
			id         TEXT PRIMARY KEY,
			company_id TEXT NOT NULL,
			status     TEXT NOT NULL DEFAULT 'queued'
		)`,
		`CREATE TABLE jobs (
			id          TEXT PRIMARY KEY,
			job_post_id TEXT NOT NULL,
			added_at    DATETIME NOT NULL
		)`,
		`CREATE TABLE job_pipeline_ledger (
			job_id        TEXT NOT NULL PRIMARY KEY,
			reason        TEXT NOT NULL,
			first_seen_at DATETIME NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("schema: %v\n%s", err, stmt)
		}
	}
	return db
}

func execT(t *testing.T, db *gorm.DB, stmt string, args ...any) {
	t.Helper()
	if err := db.Exec(stmt, args...).Error; err != nil {
		t.Fatalf("exec %q: %v", stmt, err)
	}
}

// closedPipelineDB returns a DB whose underlying *sql.DB is closed — every
// query surfaces a DB error, exercising the repository error-mapping branch.
func closedPipelineDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newPipelineTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

func asPipelineSvcErr(t *testing.T, err error) *domainerrs.ServiceError {
	t.Helper()
	var se *domainerrs.ServiceError
	if !errors.As(err, &se) {
		t.Fatalf("error %v is not a *ServiceError", err)
	}
	return se
}

// ---------- GetStats ----------

func TestPipelineRepo_GetStats_HappyPath(t *testing.T) {
	t.Parallel()
	db := newPipelineTestDB(t)
	now := time.Now().UTC()
	recent := now.Add(-1 * time.Hour)
	stale := now.Add(-48 * time.Hour)

	execT(t, db, `INSERT INTO companies (id, name) VALUES ('c1','Acme'), ('c2','Beta'), ('c3','Gamma')`)
	// c1 active, c2 inactive, c3 has no scan-settings row at all.
	execT(t, db, `INSERT INTO company_scan_settings (company_id, active) VALUES ('c1', 1), ('c2', 0)`)

	execT(t, db, `INSERT INTO company_scan_queue (id, company_id, status) VALUES
		('q1','c1','queued'), ('q2','c2','queued'), ('q3','c3','running'), ('q4','c1','done')`)

	execT(t, db, `INSERT INTO jobs (id, job_post_id, added_at) VALUES
		('j1','p1', ?), ('j2','p2', ?)`, recent, stale)

	execT(t, db, `INSERT INTO job_pipeline_ledger (job_id, reason, first_seen_at) VALUES
		('p1', 'fetch', ?),
		('p3', 'fetch', ?),
		('p2', 'filtered', ?),
		('p4', 'filtered', ?),
		('p5', 'error', ?),
		('p6', 'error', ?)`,
		recent, recent, // p1 already in jobs (progressed), p3 not in jobs (awaiting_filter)
		recent, recent, // p2 already in jobs (progressed), p4 not in jobs (awaiting_enrichment)
		recent, stale, // p5 recent error counts, p6 stale error does not
	)

	svc := pipelinesvc.NewPipelineRepositoryService(pipelinesvc.NewPipelineStore(db))
	stats, err := svc.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}

	want := pipelinedto.PipelineStats{
		TotalCompanies:     3,
		ActiveCompanies:    1, // only c1
		Queued:             2, // q1, q2
		Crawling:           1, // q3
		AwaitingFilter:     1, // p3 (fetch, no matching jobs row)
		AwaitingEnrichment: 1, // p4 (filtered, no matching jobs row)
		NewJobs24h:         1, // j1 only (j2 is 48h old)
		Errors24h:          1, // p5 only (p6 is 48h old)
	}
	if stats != want {
		t.Errorf("GetStats = %+v, want %+v", stats, want)
	}
}

func TestPipelineRepo_GetStats_EmptyDB(t *testing.T) {
	t.Parallel()
	db := newPipelineTestDB(t)
	svc := pipelinesvc.NewPipelineRepositoryService(pipelinesvc.NewPipelineStore(db))
	stats, err := svc.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats != (pipelinedto.PipelineStats{}) {
		t.Errorf("GetStats on empty DB = %+v, want all zeros", stats)
	}
}

func TestPipelineRepo_GetStats_DBError(t *testing.T) {
	t.Parallel()
	svc := pipelinesvc.NewPipelineRepositoryService(pipelinesvc.NewPipelineStore(closedPipelineDB(t)))
	_, err := svc.GetStats(context.Background())
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
	se := asPipelineSvcErr(t, err)
	if !errors.Is(se, domainerrs.ErrInternal) {
		t.Errorf("want ErrInternal, got code=%q", se.Code)
	}
}

// ---------- TriggerRetry ----------

func TestPipelineRepo_TriggerRetry_AlwaysNoOp(t *testing.T) {
	t.Parallel()
	db := newPipelineTestDB(t)
	svc := pipelinesvc.NewPipelineRepositoryService(pipelinesvc.NewPipelineStore(db))

	cases := []pipelinedto.TriggerRetryInput{
		{Worker: "filter", Tier: "1m"},
		{Worker: "enricher", Tier: "dlq"},
	}
	for _, in := range cases {
		result, err := svc.TriggerRetry(context.Background(), in)
		if err != nil {
			t.Fatalf("TriggerRetry(%+v): %v", in, err)
		}
		if result.Drained != 0 {
			t.Errorf("TriggerRetry(%+v).Drained = %d, want 0", in, result.Drained)
		}
	}
}
