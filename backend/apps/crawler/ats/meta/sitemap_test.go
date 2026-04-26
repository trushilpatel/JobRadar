package meta

import "testing"

// ── parseSitemap ─────────────────────────────────────────────────────────────

func TestParseSitemap_ExtractsAllUniqueIDsInOrder(t *testing.T) {
	body := []byte(`<?xml version="1.0"?>
<urlset>
  <url><loc>https://www.metacareers.com/profile/job_details/12345</loc></url>
  <url><loc>https://www.metacareers.com/profile/job_details/67890</loc></url>
  <url><loc>https://www.metacareers.com/profile/job_details/11111</loc></url>
</urlset>`)
	got := parseSitemap(body)
	want := []string{"12345", "67890", "11111"}
	if len(got) != len(want) {
		t.Fatalf("got %d, want %d (%v)", len(got), len(want), got)
	}
	for i, id := range want {
		if got[i] != id {
			t.Errorf("got[%d]=%q, want %q", i, got[i], id)
		}
	}
}

func TestParseSitemap_DedupesRepeats(t *testing.T) {
	// Real-world quirk — a regenerated sitemap occasionally has the same id
	// listed twice. Keep first occurrence, drop the rest.
	body := []byte(`<urlset>
  <url><loc>https://www.metacareers.com/profile/job_details/100</loc></url>
  <url><loc>https://www.metacareers.com/profile/job_details/200</loc></url>
  <url><loc>https://www.metacareers.com/profile/job_details/100</loc></url>
</urlset>`)
	got := parseSitemap(body)
	if len(got) != 2 {
		t.Errorf("got %d, want 2 deduped (%v)", len(got), got)
	}
	if got[0] != "100" || got[1] != "200" {
		t.Errorf("dedup must preserve first-occurrence order: %v", got)
	}
}

func TestParseSitemap_IgnoresNonJobLocs(t *testing.T) {
	// Sitemaps can mix job URLs with marketing pages; we only want the jobs.
	body := []byte(`<urlset>
  <url><loc>https://www.metacareers.com/about</loc></url>
  <url><loc>https://www.metacareers.com/profile/job_details/555</loc></url>
  <url><loc>https://www.metacareers.com/teams</loc></url>
</urlset>`)
	got := parseSitemap(body)
	if len(got) != 1 || got[0] != "555" {
		t.Errorf("got %v, want [555]", got)
	}
}

func TestParseSitemap_EmptyBody(t *testing.T) {
	if got := parseSitemap([]byte("")); len(got) != 0 {
		t.Errorf("empty body should produce 0 ids, got %v", got)
	}
}

func TestParseSitemap_HandlesMixedCaseLocTags(t *testing.T) {
	body := []byte(`<urlset>
  <Url><LOC>https://www.metacareers.com/profile/job_details/42</LOC></Url>
</urlset>`)
	got := parseSitemap(body)
	if len(got) != 1 || got[0] != "42" {
		t.Errorf("regex must be case-insensitive on the <loc> tag: got %v", got)
	}
}
