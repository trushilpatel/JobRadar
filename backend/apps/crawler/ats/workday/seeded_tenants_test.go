package workday

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
)

// seededTenant is one row from testdata/sample_tenants.txt — a curated subset
// of migration 055's 348 Workday tenants used to validate the live API path.
// We do NOT iterate all 348 because (a) Workday rate-limits aggressively and
// (b) a single dark or rate-limited tenant should not fail the gate for the
// others.
type seededTenant struct {
	id   string
	host string
	site string
	name string
}

// loadSampleTenants reads testdata/sample_tenants.txt — each line is
// `id|host|site|name`. Same pipe-delimited format as the migration generator.
func loadSampleTenants(t *testing.T) []seededTenant {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "sample_tenants.txt"))
	if err != nil {
		t.Fatalf("open testdata/sample_tenants.txt: %v", err)
	}
	defer func() { _ = f.Close() }()

	var out []seededTenant
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			t.Fatalf("bad row in sample_tenants.txt: %q", line)
		}
		out = append(out, seededTenant{
			id:   parts[0],
			host: parts[1],
			site: parts[2],
			name: parts[3],
		})
	}
	if err := scan.Err(); err != nil {
		t.Fatalf("scan testdata/sample_tenants.txt: %v", err)
	}
	return out
}

// TestSeededTenants_MatchMigration enforces that every sample tenant in
// testdata/sample_tenants.txt also appears in migration 055. If the migration
// drops a tenant the test sample still references, this fails so the live
// integration test below stays in lock-step with the seed data.
func TestSeededTenants_MatchMigration(t *testing.T) {
	mig := filepath.Join("..", "..", "..", "..", "migrations", "055_seed_workday_companies.sql")
	b, err := os.ReadFile(mig)
	if err != nil {
		t.Fatalf("read %s: %v", mig, err)
	}
	body := string(b)

	for _, ten := range loadSampleTenants(t) {
		needle := "('" + ten.id + "',"
		if !strings.Contains(body, needle) {
			t.Errorf("tenant %q is in sample_tenants.txt but not in migration 055", ten.id)
		}
		if !strings.Contains(body, ten.host) {
			t.Errorf("tenant %q host %q not found in migration 055 adapter_config", ten.id, ten.host)
		}
	}
}

// TestSeededTenants_AllReachable exercises FetchJobsWithConfig against a
// curated sample of live Workday boards. Each call goes through the real
// adapter with {host, site} from the seed migration — the same code path the
// crawler engine uses at scan time.
//
// Gated by TEST_REAL=1. Concurrency bounded to 3 because Workday is the most
// rate-limit-sensitive ATS we integrate with (CloudFront starts returning 403
// around 10 rps from a single client).
//
// A tenant that returns 0 jobs is NOT a failure — boards can legitimately have
// no current openings. A non-nil error IS counted as a failure. We require at
// least half the sample to succeed; below that means something is
// systematically broken (rate limit, auth pattern change, pagination regression).
func TestSeededTenants_AllReachable(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping live Workday reachability test; set TEST_REAL=1 to run")
	}

	tenants := loadSampleTenants(t)
	if len(tenants) == 0 {
		t.Fatal("no tenants loaded from testdata/sample_tenants.txt")
	}

	const parallelism = 3
	w := New()
	sem := make(chan struct{}, parallelism)

	type failure struct {
		id  string
		err error
	}
	type success struct {
		id   string
		jobs int
	}

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		failures  []failure
		successes []success
		totalJobs int64
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	for _, t0 := range tenants {
		tn := t0
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			cfg := map[string]any{
				"host":              tn.host,
				"site":              tn.site,
				"max_jobs_per_scan": 20, // keep the live test cheap
			}
			cfgBytes, _ := json.Marshal(cfg)

			resp, err := w.FetchJobsWithConfig(ctx, tn.id, cfgBytes)
			if err != nil {
				mu.Lock()
				failures = append(failures, failure{id: tn.id, err: err})
				mu.Unlock()
				return
			}
			mu.Lock()
			successes = append(successes, success{id: tn.id, jobs: len(resp.Jobs)})
			mu.Unlock()
			atomic.AddInt64(&totalJobs, int64(len(resp.Jobs)))
		}()
	}
	wg.Wait()

	for _, f := range failures {
		t.Logf("FAIL %s: %v", f.id, f.err)
	}
	for _, s := range successes {
		t.Logf("OK   %s: %d jobs", s.id, s.jobs)
	}

	required := len(tenants) / 2
	if len(successes) < required {
		t.Fatalf("only %d of %d Workday sample tenants succeeded — expected at least %d. "+
			"Either Workday API rate-limited heavily, or the adapter regressed.",
			len(successes), len(tenants), required)
	}
	t.Logf("OK: %d/%d tenants reachable, %d total open jobs (sample, max_jobs_per_scan=20)",
		len(successes), len(tenants), totalJobs)
}

// TestFetchJobContent_LiveDetailRoundTrip pulls one job from the live NVIDIA
// listing and exercises the detail-fetch path. This complements
// TestSeededTenants_AllReachable by also covering the analyzer's content
// fetch endpoint — the same call the job_post_analyzer makes for every job.
func TestFetchJobContent_LiveDetailRoundTrip(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping live Workday detail test; set TEST_REAL=1 to run")
	}

	w := New()
	cfg := json.RawMessage(`{"host":"nvidia.wd5.myworkdayjobs.com","site":"NVIDIAExternalCareerSite","max_jobs_per_scan":5}`)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := w.FetchJobsWithConfig(ctx, "nvidia", cfg)
	if err != nil {
		t.Fatalf("FetchJobsWithConfig(nvidia): %v", err)
	}
	if len(resp.Jobs) == 0 {
		t.Fatal("expected >=1 NVIDIA job for the detail round-trip")
	}

	first := resp.Jobs[0]
	if first.URL == "" {
		t.Fatal("listing item must have URL set (analyzer derives detail URL from it)")
	}

	raw := types.RawJob{
		ID:      first.ID,
		URL:     first.URL,
		Adapter: "workday",
	}
	detail, err := w.FetchJobContent(ctx, raw)
	if err != nil {
		t.Fatalf("FetchJobContent(%s): %v", first.URL, err)
	}
	if detail.Title == "" {
		t.Error("detail Title should be non-empty")
	}
	if detail.Description == "" {
		t.Error("detail Description should be non-empty (HTML-stripped plain text)")
	}
	t.Logf("OK: detail for %s — title=%q, %d chars description",
		first.ID, detail.Title, len(detail.Description))
}
