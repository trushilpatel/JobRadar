package meta

import (
	"regexp"
)

// reSitemapLoc captures every <loc>…</loc> URL in the sitemap.
// The sitemap is a flat list of <loc> tags so a regex parser is simpler
// and faster than pulling in encoding/xml.
var reSitemapLoc = regexp.MustCompile(`(?i)<loc>([^<]+)</loc>`)

// reJobIDFromURL extracts the numeric job id from a /profile/job_details/{id}
// URL. Used during sitemap parsing.
var reJobIDFromURL = regexp.MustCompile(`/profile/job_details/(\d+)`)

// parseSitemap extracts unique numeric job IDs from a sitemap.xml body.
// Order-preserving: the first occurrence of each ID wins. Pulled out so
// the unit tests can share the parsing path without network I/O.
func parseSitemap(data []byte) []string {
	matches := reSitemapLoc.FindAllSubmatch(data, -1)
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
