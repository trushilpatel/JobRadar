package ats

import (
	"html"
	"regexp"
	"strings"
)

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

// StripHTML removes all HTML tags, decodes entities, and collapses whitespace.
// Identical contract to types.StripHTML in the backend crawler.
func StripHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}
