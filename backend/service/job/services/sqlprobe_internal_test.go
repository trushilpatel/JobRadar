package services

// Regression checks for the Postgres aliasing bug uncovered by the jobs API
// audit.
//
// GORM's `Joins("Company")` aliases the joined `companies` table as
// `"Company"`. Any WHERE/ORDER reference to the unaliased name fails on
// Postgres with:
//
//   ERROR: invalid reference to FROM-clause entry for table "companies"
//   HINT:  Perhaps you meant to reference the table alias "Company".
//
// That single mismatch breaks the entire jobs feed: search, the companies
// filter, and the sort-by-company path. These tests run the actual store
// methods through GORM's DryRun mode (against a sqlite dialector) and inspect
// the emitted SQL via a custom logger — sqlite quotes identifiers with
// backticks instead of double-quotes, but the alias structure is identical
// and we only care that *our* string literals reference the alias correctly.

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// sqlCapture is a GORM logger that captures every emitted SQL statement.
type sqlCapture struct {
	gormlogger.Interface
	mu  sync.Mutex
	sql []string
}

func (c *sqlCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sql = append(c.sql, sql)
}

// LogMode satisfies the logger.Interface.
func (c *sqlCapture) LogMode(gormlogger.LogLevel) gormlogger.Interface { return c }
func (c *sqlCapture) Info(context.Context, string, ...any)             {}
func (c *sqlCapture) Warn(context.Context, string, ...any)             {}
func (c *sqlCapture) Error(context.Context, string, ...any)            {}

func newCapturingDB(t *testing.T) (*gorm.DB, *sqlCapture) {
	t.Helper()
	cap := &sqlCapture{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DryRun: true,
		Logger: cap,
	})
	if err != nil {
		t.Fatalf("open dryrun: %v", err)
	}
	return db, cap
}

// TestSQL_GetJobs_AliasReferences asserts every references to the joined
// companies table inside GetJobs uses the `"Company"` alias.
func TestSQL_GetJobs_AliasReferences(t *testing.T) {
	db, cap := newCapturingDB(t)
	store := &jobStore{db: db}
	filter := jobdto.JobFilter{
		Search:    "engineer",
		Companies: []string{"Google"},
		SortBy:    "company",
		SortDir:   "desc",
	}
	_, _, _ = store.GetJobs(context.Background(), filter, "", 20)

	assertAliasOnly(t, cap.sql, "GetJobs")
}

// TestSQL_GetJobsByStatus_AliasReferences covers the user_job_state-driven
// list path used for saved/applied/dismissed/interviewing/offer/rejected tabs.
func TestSQL_GetJobsByStatus_AliasReferences(t *testing.T) {
	db, cap := newCapturingDB(t)
	repo := &userJobStateRepository{db: db}
	filter := jobdto.JobFilter{
		Status:    "saved",
		Search:    "engineer",
		Companies: []string{"Google"},
		SortBy:    "company",
		SortDir:   "asc",
	}
	_, _, _ = repo.GetJobsByStatus(context.Background(), "user-1", filter, "", 20)

	assertAliasOnly(t, cap.sql, "GetJobsByStatus")
}

// TestSQL_CursorPredicate_CompanySort exercises the company-sort cursor
// predicate on a second-page request. The predicate must reference the alias.
func TestSQL_CursorPredicate_CompanySort(t *testing.T) {
	db, cap := newCapturingDB(t)
	store := &jobStore{db: db}
	cur := encodeCursor("company", "asc", "Google", time.Now(), "j-1")
	filter := jobdto.JobFilter{SortBy: "company", SortDir: "asc"}
	_, _, _ = store.GetJobs(context.Background(), filter, cur, 20)

	assertAliasOnly(t, cap.sql, "GetJobs with company cursor")
}

// assertAliasOnly fails the test if any captured SQL still references the
// unaliased `companies.name` (or other unaliased columns) in a position that
// would error against the aliased join.
func assertAliasOnly(t *testing.T, captured []string, label string) {
	t.Helper()
	if len(captured) == 0 {
		t.Fatalf("%s: no SQL captured", label)
	}
	for _, sql := range captured {
		// Strip the SELECT clause — GORM's auto-selected projection legitimately
		// uses `Company` for join columns; the unaliased "companies" doesn't
		// appear there. The risk surface is WHERE/ORDER/HAVING.
		whereOnwards := sql
		if idx := strings.Index(sql, "WHERE"); idx >= 0 {
			whereOnwards = sql[idx:]
		} else if idx := strings.Index(sql, "ORDER BY"); idx >= 0 {
			whereOnwards = sql[idx:]
		}
		if strings.Contains(whereOnwards, "companies.name") {
			t.Errorf("%s: WHERE/ORDER references unaliased companies.name (alias regression): %s", label, sql)
		}
		// Sanity: every test sets a search or a company-sort that should produce
		// an alias reference. Catches the case where we accidentally erase the
		// clauses.
		if !strings.Contains(whereOnwards, `"Company".name`) {
			t.Errorf(`%s: expected "Company".name in WHERE/ORDER, got: %s`, label, sql)
		}
	}
}
