package meta

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

// sitemapURL is the public sitemap of every active Meta job posting.
// Advertised in https://www.metacareers.com/robots.txt under "Sitemap:".
//
// Verified: ~115 KB plain XML, ~895 unique <loc> entries, all of the form
// /profile/job_details/{numeric_id}. No auth, no JS, no rotating tokens —
// the sitemap is the official crawler contract.
//
// Declared as `var` (not `const`) so tests can point fetchSitemap at a
// httptest.Server via overrideSitemapURL.
var sitemapURL = "https://www.metacareers.com/jobsearch/sitemap.xml"

// overrideSitemapURL is a test-only seam — production code should leave
// sitemapURL alone. Pulled out of the test files so we don't sprinkle
// _test.go internals across multiple files.
func overrideSitemapURL(url string) { sitemapURL = url }

// reSitemapLoc captures every <loc>…</loc> URL in the sitemap. The sitemap
// is a flat list of <loc> tags so a regex parser is simpler (and faster)
// than pulling in encoding/xml.
var reSitemapLoc = regexp.MustCompile(`(?i)<loc>([^<]+)</loc>`)

// reJobIDFromURL extracts the numeric job id from a /profile/job_details/{id}
// URL. Used during sitemap parsing.
var reJobIDFromURL = regexp.MustCompile(`/profile/job_details/(\d+)`)

// fetchSitemap pulls the sitemap and returns the deduped, in-order list of
// numeric job IDs (without the "meta-" prefix). Callers add the prefix when
// publishing pipeline items.
func (m *MetaAdapter) fetchSitemap(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sitemapURL, nil)
	if err != nil {
		return nil, fmt.Errorf("meta/sitemap: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/xml,text/xml,*/*;q=0.8")
	req.Header.Set("Accept-Language", acceptLangHeader)
	// Meta's CDN validates the Sec-Fetch/sec-ch-ua headers on the sitemap
	// endpoint; requests without them (Chrome UA + basic headers only) get 400.
	req.Header.Set("sec-ch-ua", `"Chromium";v="121", "Not A(Brand";v="99", "Google Chrome";v="121"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("meta/sitemap: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("meta/sitemap: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("meta/sitemap: read body: %w", err)
	}
	return parseSitemap(body), nil
}

// parseSitemap extracts unique numeric job IDs from a sitemap.xml body.
// Order-preserving: the first occurrence of each ID wins. Pulled out so
// fetchSitemap and the unit tests can share the parsing path.
func parseSitemap(body []byte) []string {
	matches := reSitemapLoc.FindAllSubmatch(body, -1)
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		idMatch := reJobIDFromURL.FindSubmatch(m[1])
		if idMatch == nil {
			continue
		}
		id := string(idMatch[1])
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
