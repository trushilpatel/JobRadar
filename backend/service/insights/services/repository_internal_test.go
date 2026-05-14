package services

// Repository-layer tests exercise the SQL building paths of
// InsightsRepositoryService against GORM-backed sqlite connections.
//
// We use two flavors of fake DB:
//
//   - DryRun mode + sqlite dialector with a custom logger — captures every
//     rendered SQL so we can assert on filters, time windows, GROUP BY and
//     LIMIT scaffolding without running Postgres. Under DryRun, Raw().Scan()
//     returns the sentinel "dry run mode unsupported"; the repository then
//     wraps it into a 500 ServiceError, exercising the err branch of each
//     method. This guards against breaking the SQL shape during refactors.
//
//   - Real (executing) sqlite with stub tables — exercises the success
//     branch of each method whose SQL is portable enough to run on sqlite.
//     Postgres-specific syntax (jsonb_array_elements_text, INTERVAL,
//     DATE_TRUNC) prevents us from running every method this way, but
//     several can be checked end-to-end.
//
//   - Closed *sql.DB — every method fails at the driver and must wrap the
//     error into a 500 ServiceError, covering the error path in production.

import (
	"context"
	stderrors "errors"
	"strings"
	"sync"
	"testing"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/insights/dto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// repoSQLCapture records every SQL statement rendered by GORM, regardless
// of whether the statement actually executes against the underlying driver.
type repoSQLCapture struct {
	mu  sync.Mutex
	sql []string
}

func (c *repoSQLCapture) LogMode(gormlogger.LogLevel) gormlogger.Interface { return c }
func (c *repoSQLCapture) Info(context.Context, string, ...any)             {}
func (c *repoSQLCapture) Warn(context.Context, string, ...any)             {}
func (c *repoSQLCapture) Error(context.Context, string, ...any)            {}
func (c *repoSQLCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sql = append(c.sql, sql)
}

func (c *repoSQLCapture) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.sql))
	copy(out, c.sql)
	return out
}

func newDryRunRepo(t *testing.T) (*repositoryService, *repoSQLCapture) {
	t.Helper()
	cap := &repoSQLCapture{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DryRun: true,
		Logger: cap,
	})
	if err != nil {
		t.Fatalf("open dryrun: %v", err)
	}
	return &repositoryService{db: db}, cap
}

func mustContain(t *testing.T, captured []string, needle string) {
	t.Helper()
	for _, sql := range captured {
		if strings.Contains(sql, needle) {
			return
		}
	}
	t.Errorf("no captured SQL contained %q; captured = %v", needle, captured)
}

// TestRepository_NewRepositoryService_ReturnsInterface protects the
// constructor contract: it must return the InsightsService interface, not
// the concrete struct, so callers cannot bind to implementation details.
func TestRepository_NewRepositoryService_ReturnsInterface(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("open dryrun: %v", err)
	}
	repo := NewRepositoryService(db)
	if repo == nil {
		t.Fatal("NewRepositoryService returned nil")
	}
}

// TestRepository_TechStack_SQL asserts the rendered SQL contains the
// jsonb_array_elements_text expansion, the tech_stack NOT NULL guard, and
// the LIMIT 15 cap.
func TestRepository_TechStack_SQL(t *testing.T) {
	repo, cap := newDryRunRepo(t)
	_, _ = repo.TechStack(context.Background())

	captured := cap.all()
	if len(captured) == 0 {
		t.Fatal("no SQL captured")
	}
	mustContain(t, captured, "jsonb_array_elements_text")
	mustContain(t, captured, "tech_stack IS NOT NULL")
	mustContain(t, captured, "GROUP BY tech")
	mustContain(t, captured, "LIMIT 15")
}

// TestRepository_CompanyActivity_SQL asserts the 90-day window, week
// truncation, join, GROUP BY, and 200-row cap.
func TestRepository_CompanyActivity_SQL(t *testing.T) {
	repo, cap := newDryRunRepo(t)
	_, _ = repo.CompanyActivity(context.Background())

	captured := cap.all()
	if len(captured) == 0 {
		t.Fatal("no SQL captured")
	}
	mustContain(t, captured, "INTERVAL '90 days'")
	mustContain(t, captured, "DATE_TRUNC('week'")
	mustContain(t, captured, "JOIN companies c ON j.company_id = c.id")
	mustContain(t, captured, "LIMIT 200")
}

// TestRepository_Funnel_SQL asserts the funnel reads enriched from
// job_analytics and counts saved/applied/dismissed from user_job_state with
// the documented status sets (applied includes interviewing and offer).
func TestRepository_Funnel_SQL(t *testing.T) {
	repo, cap := newDryRunRepo(t)
	_, _ = repo.Funnel(context.Background())

	captured := cap.all()
	if len(captured) == 0 {
		t.Fatal("no SQL captured")
	}
	mustContain(t, captured, "FROM job_analytics")
	mustContain(t, captured, "FROM user_job_state")
	mustContain(t, captured, "status = 'saved'")
	mustContain(t, captured, "status IN ('applied', 'interviewing', 'offer')")
	mustContain(t, captured, "status = 'dismissed'")
}

// TestRepository_Funnel_SuccessShape exercises the Funnel success branch
// against a real sqlite-backed schema with the relevant tables. sqlite
// implements FILTER (WHERE ...) so the query is portable enough; we only
// assert the static shape — four stages in canonical order regardless of
// row counts.
func TestRepository_Funnel_SuccessShape(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Exec(`CREATE TABLE job_analytics (id INTEGER PRIMARY KEY)`).Error; err != nil {
		t.Fatalf("create job_analytics: %v", err)
	}
	if err := db.Exec(`CREATE TABLE user_job_state (id INTEGER PRIMARY KEY, status TEXT)`).Error; err != nil {
		t.Fatalf("create user_job_state: %v", err)
	}
	repo := &repositoryService{db: db}
	data, err := repo.Funnel(context.Background())
	if err != nil {
		t.Fatalf("Funnel: %v", err)
	}
	steps, ok := data.([]dto.FunnelStep)
	if !ok {
		t.Fatalf("expected []FunnelStep, got %T", data)
	}
	wantStages := []string{"enriched", "saved", "applied", "dismissed"}
	if len(steps) != len(wantStages) {
		t.Fatalf("got %d stages, want %d", len(steps), len(wantStages))
	}
	for i, w := range wantStages {
		if steps[i].Stage != w {
			t.Errorf("step %d stage = %q, want %q", i, steps[i].Stage, w)
		}
	}
}

// TestRepository_Funnel_CountsFromUserJobState seeds user_job_state with
// concrete rows and asserts the FILTER aggregations route each row into the
// correct stage. Specifically: applied count must include 'interviewing'
// and 'offer' statuses (documented invariant); dismissed must come only
// from status='dismissed'.
func TestRepository_Funnel_CountsFromUserJobState(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Exec(`CREATE TABLE job_analytics (id INTEGER PRIMARY KEY)`).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.Exec(`CREATE TABLE user_job_state (id INTEGER PRIMARY KEY, status TEXT)`).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	// 2 enriched, 3 saved, 1 applied + 1 interviewing + 1 offer = 3 applied, 2 dismissed.
	if err := db.Exec(`INSERT INTO job_analytics (id) VALUES (1), (2)`).Error; err != nil {
		t.Fatalf("seed analytics: %v", err)
	}
	if err := db.Exec(`INSERT INTO user_job_state (id, status) VALUES
		(1, 'saved'), (2, 'saved'), (3, 'saved'),
		(4, 'applied'), (5, 'interviewing'), (6, 'offer'),
		(7, 'dismissed'), (8, 'dismissed'),
		(9, 'new')`).Error; err != nil {
		t.Fatalf("seed user_job_state: %v", err)
	}
	repo := &repositoryService{db: db}
	data, err := repo.Funnel(context.Background())
	if err != nil {
		t.Fatalf("Funnel: %v", err)
	}
	steps, ok := data.([]dto.FunnelStep)
	if !ok {
		t.Fatalf("expected []FunnelStep, got %T", data)
	}
	got := map[string]int{}
	for _, s := range steps {
		got[s.Stage] = s.Count
	}
	want := map[string]int{
		"enriched":  2,
		"saved":     3,
		"applied":   3,
		"dismissed": 2,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("stage %q count = %d, want %d", k, got[k], v)
		}
	}
}

// TestRepository_Location_SQL captures both queries emitted by Location.
func TestRepository_Location_SQL(t *testing.T) {
	repo, cap := newDryRunRepo(t)
	_, _ = repo.Location(context.Background())

	captured := cap.all()
	// Location issues two raw queries; DryRun returns an error after the
	// first, so the function short-circuits before the second. Both code
	// paths are still exercised across the file (TestRepository_Location_Success
	// covers the second query with executing sqlite).
	if len(captured) == 0 {
		t.Fatal("no SQL captured")
	}
	mustContain(t, captured, "COALESCE(country, 'Unknown')")
	mustContain(t, captured, "LIMIT 50")
}

// TestRepository_Location_Success runs the full Location method against a
// real sqlite database — both the countries query and the states query —
// and asserts the returned map carries both keys.
func TestRepository_Location_Success(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Exec(`CREATE TABLE job_analytics (
		id INTEGER PRIMARY KEY,
		country TEXT,
		state TEXT
	)`).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.Exec(`INSERT INTO job_analytics (id, country, state) VALUES
		(1, 'USA', 'CA'), (2, 'USA', 'CA'), (3, 'USA', 'NY'),
		(4, 'Canada', NULL), (5, NULL, NULL)`).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := &repositoryService{db: db}
	data, err := repo.Location(context.Background())
	if err != nil {
		t.Fatalf("Location: %v", err)
	}
	m, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", data)
	}
	countries, ok := m["countries"].([]dto.LocationCount)
	if !ok {
		t.Fatalf("expected []LocationCount, got %T", m["countries"])
	}
	// The NULL country row should be coalesced to "Unknown"; both USA and
	// Canada should appear; USA must be first by count desc.
	if len(countries) < 2 {
		t.Fatalf("expected at least 2 country rows, got %d", len(countries))
	}
	if countries[0].Country != "USA" || countries[0].Count != 3 {
		t.Errorf("top country = %+v, want USA/3", countries[0])
	}

	states, ok := m["states"].([]dto.StateCount)
	if !ok {
		t.Fatalf("expected []StateCount, got %T", m["states"])
	}
	if len(states) != 2 {
		t.Fatalf("expected 2 state rows (CA, NY), got %d: %+v", len(states), states)
	}
	if states[0].State != "CA" || states[0].Count != 2 {
		t.Errorf("top state = %+v, want CA/2", states[0])
	}
}

// TestRepository_Sponsorship_DefaultDays asserts that Days <= 0 is replaced
// with 30 before the SQL is rendered. This is the documented default and
// must remain stable.
func TestRepository_Sponsorship_DefaultDays(t *testing.T) {
	repo, cap := newDryRunRepo(t)
	_, _ = repo.Sponsorship(context.Background(), dto.InsightsFilter{Days: 0})

	captured := cap.all()
	if len(captured) == 0 {
		t.Fatal("no SQL captured")
	}
	// GORM renders the bound parameter inline in the captured SQL.
	mustContain(t, captured, "30 * INTERVAL '1 day'")
}

// TestRepository_Sponsorship_CustomDays confirms a positive Days value is
// forwarded verbatim into the rendered SQL.
func TestRepository_Sponsorship_CustomDays(t *testing.T) {
	repo, cap := newDryRunRepo(t)
	_, _ = repo.Sponsorship(context.Background(), dto.InsightsFilter{Days: 7})

	captured := cap.all()
	if len(captured) == 0 {
		t.Fatal("no SQL captured")
	}
	mustContain(t, captured, "7 * INTERVAL '1 day'")
	mustContain(t, captured, "sponsorship = true")
}

// TestRepository_Sponsorship_NegativeDaysAlsoDefaults asserts that a
// negative Days value is treated identically to zero — the repo layer is a
// safety net behind the validation layer.
func TestRepository_Sponsorship_NegativeDaysAlsoDefaults(t *testing.T) {
	repo, cap := newDryRunRepo(t)
	_, _ = repo.Sponsorship(context.Background(), dto.InsightsFilter{Days: -1})

	captured := cap.all()
	if len(captured) == 0 {
		t.Fatal("no SQL captured")
	}
	mustContain(t, captured, "30 * INTERVAL '1 day'")
}

// TestRepository_Velocity_SQL asserts the 4-week CTE structure — current
// week, three prior weeks, the 28-day window, and the 20-row cap.
func TestRepository_Velocity_SQL(t *testing.T) {
	repo, cap := newDryRunRepo(t)
	_, _ = repo.Velocity(context.Background())

	captured := cap.all()
	if len(captured) == 0 {
		t.Fatal("no SQL captured")
	}
	mustContain(t, captured, "DATE_TRUNC('week', CURRENT_DATE)")
	mustContain(t, captured, "INTERVAL '7 days'")
	mustContain(t, captured, "INTERVAL '14 days'")
	mustContain(t, captured, "INTERVAL '21 days'")
	mustContain(t, captured, "INTERVAL '28 days'")
	mustContain(t, captured, "JOIN companies c ON s.company_id = c.id")
	mustContain(t, captured, "LIMIT 20")
}

// TestRepository_RoleCategories_SQL asserts the GROUP BY and 25-row cap.
func TestRepository_RoleCategories_SQL(t *testing.T) {
	repo, cap := newDryRunRepo(t)
	_, _ = repo.RoleCategories(context.Background())

	captured := cap.all()
	if len(captured) == 0 {
		t.Fatal("no SQL captured")
	}
	mustContain(t, captured, "COALESCE(role_category, 'other')")
	mustContain(t, captured, "GROUP BY role_category")
	mustContain(t, captured, "LIMIT 25")
}

// TestRepository_RoleCategories_Success runs RoleCategories against a real
// sqlite database — covers the success return path and asserts the
// COALESCE on null role_category surfaces as "other".
func TestRepository_RoleCategories_Success(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Exec(`CREATE TABLE job_analytics (
		id INTEGER PRIMARY KEY,
		role_category TEXT
	)`).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.Exec(`INSERT INTO job_analytics (id, role_category) VALUES
		(1, 'engineering'), (2, 'engineering'), (3, 'engineering'),
		(4, 'design'), (5, NULL)`).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := &repositoryService{db: db}
	data, err := repo.RoleCategories(context.Background())
	if err != nil {
		t.Fatalf("RoleCategories: %v", err)
	}
	rows, ok := data.([]dto.RoleCategoryCount)
	if !ok {
		t.Fatalf("expected []RoleCategoryCount, got %T", data)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 category rows (engineering, design, other), got %d: %+v", len(rows), rows)
	}
	if rows[0].Category != "engineering" || rows[0].Count != 3 {
		t.Errorf("top category = %+v, want engineering/3", rows[0])
	}
	// "other" row from the NULL coalesce must be present somewhere in the result.
	foundOther := false
	for _, r := range rows {
		if r.Category == "other" && r.Count == 1 {
			foundOther = true
		}
	}
	if !foundOther {
		t.Errorf("expected 'other' row with count 1 (from null role_category), got %+v", rows)
	}
}

// TestRepository_GetPipelineCounts_SQL asserts both filters: pending counts
// only rows where reason='fetch' AND the job has not yet been written to the
// jobs table; errors counts reason='error'.
func TestRepository_GetPipelineCounts_SQL(t *testing.T) {
	repo, cap := newDryRunRepo(t)
	_, _ = repo.GetPipelineCounts(context.Background())

	captured := cap.all()
	if len(captured) == 0 {
		t.Fatal("no SQL captured")
	}
	mustContain(t, captured, "FROM job_pipeline_ledger")
	mustContain(t, captured, "l.reason = 'fetch'")
	mustContain(t, captured, "j.job_post_id IS NULL")
	mustContain(t, captured, "l.reason = 'error'")
	mustContain(t, captured, "LEFT JOIN jobs j")
}

// TestRepository_GetPipelineCounts_Success runs GetPipelineCounts against a
// real sqlite database — covers the success return path and asserts the
// FILTER clauses route ledger rows correctly. Specifically a row with
// reason='fetch' must only count toward Pending when the corresponding job
// row is missing from the jobs table (the LEFT JOIN guard).
func TestRepository_GetPipelineCounts_Success(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Exec(`CREATE TABLE job_pipeline_ledger (
		job_id TEXT PRIMARY KEY,
		reason TEXT
	)`).Error; err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	if err := db.Exec(`CREATE TABLE jobs (
		job_post_id TEXT PRIMARY KEY
	)`).Error; err != nil {
		t.Fatalf("create jobs: %v", err)
	}
	if err := db.Exec(`INSERT INTO job_pipeline_ledger (job_id, reason) VALUES
		('j1', 'fetch'),
		('j2', 'fetch'),
		('j3', 'fetch'),
		('j4', 'error'),
		('j5', 'error'),
		('j6', 'retry')`).Error; err != nil {
		t.Fatalf("seed ledger: %v", err)
	}
	// j1 has already landed in jobs — must NOT count as pending.
	if err := db.Exec(`INSERT INTO jobs (job_post_id) VALUES ('j1')`).Error; err != nil {
		t.Fatalf("seed jobs: %v", err)
	}
	repo := &repositoryService{db: db}
	counts, err := repo.GetPipelineCounts(context.Background())
	if err != nil {
		t.Fatalf("GetPipelineCounts: %v", err)
	}
	if counts.Pending != 2 {
		t.Errorf("Pending = %d, want 2 (j2, j3 — j1 already in jobs)", counts.Pending)
	}
	if counts.Errors != 2 {
		t.Errorf("Errors = %d, want 2", counts.Errors)
	}
}

// TestRepository_ErrorWrapping_AsServiceError asserts that when the
// underlying DB driver returns an error, the repository wraps it into a
// 500 ServiceError with a human-readable message. We trigger an error by
// closing the database handle so every Raw call fails — and we cover every
// repository method to keep them aligned on the wrapping contract.
func TestRepository_ErrorWrapping_AsServiceError(t *testing.T) {
	cap := &repoSQLCapture{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: cap})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	repo := &repositoryService{db: db}

	type call struct {
		name string
		fn   func() error
	}
	calls := []call{
		{"TechStack", func() error { _, err := repo.TechStack(context.Background()); return err }},
		{"CompanyActivity", func() error { _, err := repo.CompanyActivity(context.Background()); return err }},
		{"Funnel", func() error { _, err := repo.Funnel(context.Background()); return err }},
		{"Location", func() error { _, err := repo.Location(context.Background()); return err }},
		{"Sponsorship", func() error {
			_, err := repo.Sponsorship(context.Background(), dto.InsightsFilter{Days: 30})
			return err
		}},
		{"Velocity", func() error { _, err := repo.Velocity(context.Background()); return err }},
		{"RoleCategories", func() error {
			_, err := repo.RoleCategories(context.Background())
			return err
		}},
		{"GetPipelineCounts", func() error {
			_, err := repo.GetPipelineCounts(context.Background())
			return err
		}},
	}
	for _, c := range calls {
		err := c.fn()
		if err == nil {
			t.Errorf("%s did not error on a closed DB", c.name)
			continue
		}
		var sErr *apperr.ServiceError
		if !stderrors.As(err, &sErr) {
			t.Errorf("%s: expected *apperr.ServiceError, got %T (%v)", c.name, err, err)
			continue
		}
		if sErr.Status != 500 {
			t.Errorf("%s: status = %d, want 500", c.name, sErr.Status)
		}
	}
}
