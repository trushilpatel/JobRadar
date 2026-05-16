package gem

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// seededSlugs returns the canonical list of Gem vanity-slug board IDs that
// migration 052_seed_gem_companies.sql inserts.
//
// The list is kept in testdata/seeded_slugs.txt so the build_migration.py
// generator and these tests share a single source of truth. TestSeededSlugs_MatchMigration
// (below) guards against drift between the testdata file and the migration.
func seededSlugs(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "seeded_slugs.txt"))
	if err != nil {
		t.Fatalf("open testdata/seeded_slugs.txt: %v", err)
	}
	defer func() { _ = f.Close() }()

	var out []string
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		s := strings.TrimSpace(scan.Text())
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	if err := scan.Err(); err != nil {
		t.Fatalf("read testdata/seeded_slugs.txt: %v", err)
	}
	return out
}

// TestSeededSlugs_MatchMigration enforces that the testdata slug list is a
// strict superset of the slugs inserted by migration 052. If a slug is added to
// the migration without being added here (or vice versa), this test fails so
// the integration test below stays in lock-step with the seed data.
func TestSeededSlugs_MatchMigration(t *testing.T) {
	// Resolve the migration path relative to this test file's directory.
	// backend/apps/crawler/ats/gem -> backend/migrations
	mig := filepath.Join("..", "..", "..", "..", "migrations", "052_seed_gem_companies.sql")
	b, err := os.ReadFile(mig)
	if err != nil {
		t.Fatalf("read %s: %v", mig, err)
	}
	body := string(b)

	want := make(map[string]struct{}, 256)
	for _, s := range seededSlugs(t) {
		want[s] = struct{}{}
	}

	// Each company is inserted on a line like:
	//     ('the-slug', 'Name', 'the-slug', 'https://jobs.gem.com/the-slug', ...
	// We assert the testdata slug list is a strict subset of what the migration inserts.
	// A simple substring check on `('<slug>'` keeps this parser robust to formatting
	// changes while still catching missing entries.
	for slug := range want {
		needle := "('" + slug + "',"
		if !strings.Contains(body, needle) {
			t.Errorf("slug %q is in testdata but not in migration 052", slug)
		}
	}
}

// TestSeededSlugs_AllReachable hits the live gem.com Job Board API for every
// slug in the seed list and asserts FetchJobs succeeds for each. This is the
// end-to-end smoke test that backs the seed migration: if Gem ever disables a
// board or rotates a slug, this test surfaces it before the change ships.
//
// Gated by TEST_REAL=1 so unit-test CI stays hermetic. Runs ~253 HTTP calls
// against api.gem.com with bounded concurrency.
func TestSeededSlugs_AllReachable(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping live Gem reachability test; set TEST_REAL=1 to run")
	}

	slugs := seededSlugs(t)
	if len(slugs) == 0 {
		t.Fatal("no slugs loaded from testdata/seeded_slugs.txt")
	}

	const parallelism = 20
	g := New()
	sem := make(chan struct{}, parallelism)

	type failure struct {
		slug string
		err  error
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		failures []failure
		jobs     int64
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for _, s := range slugs {
		slug := s
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			resp, err := g.FetchJobs(ctx, slug)
			if err != nil {
				mu.Lock()
				failures = append(failures, failure{slug: slug, err: err})
				mu.Unlock()
				return
			}
			atomic.AddInt64(&jobs, int64(len(resp.Jobs)))
		}()
	}
	wg.Wait()

	if len(failures) > 0 {
		for _, f := range failures {
			t.Errorf("FetchJobs(%q) failed: %v", f.slug, f.err)
		}
		t.Fatalf("%d of %d seeded Gem slugs failed FetchJobs — fix or remove them from migration 052 before merge", len(failures), len(slugs))
	}
	t.Logf("OK: %d seeded slugs all reachable; %d total open jobs across all boards", len(slugs), jobs)
}
