package meta

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	metaapi "github.com/trushilpatel/jobradar/jobingest/adapters/ats/meta/api"
)

// reJSONLD finds every <script type="application/ld+json"> block in a Meta
// detail page. Greedy match across newlines because the JSON-LD payload
// contains real newlines inside quoted strings. We pick the JobPosting one.
var reJSONLD = regexp.MustCompile(
	`(?is)<script\s+type=["']application/ld\+json["'][^>]*>(.*?)</script>`,
)

// extractJobPosting pulls the first <script type="application/ld+json"> block
// whose @type is "JobPosting" out of a Meta detail-page HTML body.
//
// Meta typically embeds exactly one JSON-LD block per detail page; we still
// scan all of them and return the first JobPosting in case Meta later adds
// other structured-data scripts (BreadcrumbList, etc.).
//
// Handles three JSON-LD shapes:
//   - plain object    { "@type": "JobPosting", … }
//   - array           [ { "@type": "JobPosting", … }, … ]
//   - @graph wrapper  { "@graph": [ { "@type": "JobPosting", … } ] }
func extractJobPosting(body []byte) (*metaapi.JobPosting, error) {
	matches := reJSONLD.FindAllSubmatch(body, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("meta/detail: no application/ld+json script block")
	}
	for _, m := range matches {
		raw := bytes.TrimSpace(m[1])

		// Plain object.
		var single metaapi.JobPosting
		if err := json.Unmarshal(raw, &single); err == nil && single.Type == "JobPosting" {
			return &single, nil
		}

		// Array of objects.
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err == nil {
			for _, item := range arr {
				var jp metaapi.JobPosting
				if err := json.Unmarshal(item, &jp); err == nil && jp.Type == "JobPosting" {
					return &jp, nil
				}
			}
		}

		// Object with @graph.
		var wrapper struct {
			Graph []json.RawMessage `json:"@graph"`
		}
		if err := json.Unmarshal(raw, &wrapper); err == nil {
			for _, item := range wrapper.Graph {
				var jp metaapi.JobPosting
				if err := json.Unmarshal(item, &jp); err == nil && jp.Type == "JobPosting" {
					return &jp, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("meta/detail: no JobPosting found in JSON-LD blocks")
}

// locations returns the deduped list of "City, Region, Country" strings
// for this posting. Multiple jobLocation entries that resolve to the same
// joined string collapse to one.
func locations(jp *metaapi.JobPosting) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, l := range jp.JobLocation {
		var parts []string
		if s := strings.TrimSpace(string(l.Address.Locality)); s != "" {
			parts = append(parts, s)
		}
		if s := strings.TrimSpace(string(l.Address.Region)); s != "" {
			parts = append(parts, s)
		}
		if s := strings.TrimSpace(string(l.Address.Country)); s != "" {
			parts = append(parts, s)
		}
		joined := strings.Join(parts, ", ")
		if joined == "" {
			continue
		}
		if _, dup := seen[joined]; dup {
			continue
		}
		seen[joined] = struct{}{}
		out = append(out, joined)
	}
	return out
}

// extractLocation returns the first location string ("City, Region, Country")
// for the posting, or the raw addressLocality fallback when Locations() is empty.
func extractLocation(jp *metaapi.JobPosting) string {
	if locs := locations(jp); len(locs) > 0 {
		return locs[0]
	}
	if len(jp.JobLocation) > 0 {
		return strings.TrimSpace(string(jp.JobLocation[0].Address.Locality))
	}
	return ""
}

// extractCountryCode walks all jobLocation entries and returns the first
// non-empty ISO alpha-2 country code found.
func extractCountryCode(jp *metaapi.JobPosting) string {
	for _, l := range jp.JobLocation {
		if code := metaCountryToCode(strings.TrimSpace(string(l.Address.Country))); code != "" {
			return code
		}
	}
	return ""
}

// metaCountryToCode converts a Meta address country value to an ISO alpha-2
// code. Meta emits "USA", "United States", plain 2-char codes (e.g. "IN"),
// or empty strings. Unknown values return "".
func metaCountryToCode(country string) string {
	c := strings.TrimSpace(country)
	switch strings.ToUpper(c) {
	case "USA", "UNITED STATES", "UNITED STATES OF AMERICA", "US":
		return "US"
	case "":
		return ""
	default:
		// Pass through if already a 2-char ISO code; ignore everything else.
		if len(c) == 2 {
			return strings.ToUpper(c)
		}
		return ""
	}
}
