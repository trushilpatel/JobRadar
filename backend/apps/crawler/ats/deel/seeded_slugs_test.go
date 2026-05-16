package deel

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

// seededSlugs returns the canonical list of Deel vanity-slug board IDs that
// migration 053_seed_deel_companies.sql inserts.
//
// The list is kept in testdata/seeded_slugs.txt so the migration generator
// and these tests share a single source of truth. TestSeededSlugs_MatchMigration
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

// TestSeededSlugs_MatchMigration enforces that every slug in the testdata file
// also appears in migration 053. If a slug is added to the migration without
// being added here (or vice versa), this test fails so the integration test
// below stays in lock-step with the seed data.
func TestSeededSlugs_MatchMigration(t *testing.T) {
	mig := filepath.Join("..", "..", "..", "..", "migrations", "053_seed_deel_companies.sql")
	b, err := os.ReadFile(mig)
	if err != nil {
		t.Fatalf("read %s: %v", mig, err)
	}
	body := string(b)

	for _, slug := range seededSlugs(t) {
		needle := "('" + slug + "',"
		if !strings.Contains(body, needle) {
			t.Errorf("slug %q is in testdata but not in migration 053", slug)
		}
	}
}

// TestSeededSlugs_AllReachable hits the live jobs.deel.com SSR/RSC endpoint
// for every slug in the seed list and asserts FetchJobs succeeds + returns
// at least one job. This is the end-to-end smoke test that backs the seed
// migration: if Deel ever disables a board or rotates a slug, this test
// surfaces it before the change ships.
//
// Gated by TEST_REAL=1 so unit-test CI stays hermetic. Concurrency is capped
// at 5 to stay well below the CloudFront rate limits we've seen in front of
// jobs.deel.com (the adapter already uses a real-browser UA for the same
// reason — see deel.go).
func TestSeededSlugs_AllReachable(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping live Deel reachability test; set TEST_REAL=1 to run")
	}

	slugs := seededSlugs(t)
	if len(slugs) == 0 {
		t.Fatal("no slugs loaded from testdata/seeded_slugs.txt")
	}

	const parallelism = 5
	d := New()
	sem := make(chan struct{}, parallelism)

	type failure struct {
		slug string
		err  error
		jobs int
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
			resp, err := d.FetchJobs(ctx, slug)
			if err != nil {
				mu.Lock()
				failures = append(failures, failure{slug: slug, err: err})
				mu.Unlock()
				return
			}
			// Deel SPA returns HTTP 200 even for nonexistent slugs; the only
			// reliable signal that a board is real is that the RSC stream
			// contains at least one job record. Tech-cut seed slugs all had
			// >=1 job on their last verification — a 0 result indicates a
			// disabled board.
			if len(resp.Jobs) == 0 {
				mu.Lock()
				failures = append(failures, failure{slug: slug, jobs: 0})
				mu.Unlock()
				return
			}
			atomic.AddInt64(&jobs, int64(len(resp.Jobs)))
		}()
	}
	wg.Wait()

	if len(failures) > 0 {
		for _, f := range failures {
			if f.err != nil {
				t.Errorf("FetchJobs(%q) failed: %v", f.slug, f.err)
			} else {
				t.Errorf("FetchJobs(%q) returned 0 jobs — board likely disabled", f.slug)
			}
		}
		t.Fatalf("%d of %d seeded Deel slugs failed FetchJobs — fix or remove them from migration 053 before merge", len(failures), len(slugs))
	}
	t.Logf("OK: %d seeded slugs all reachable; %d total open jobs across all boards", len(slugs), jobs)
}
