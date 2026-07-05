package techtag_test

import (
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/service/job/techtag"
)

// TestClean_EmptyInputReturnsEmpty — Clean returns empty slice when there is
// nothing to normalize. The result must be non-nil with len == 0.
func TestClean_EmptyInputReturnsEmpty(t *testing.T) {
	got := techtag.Clean(nil)
	if got == nil {
		t.Fatalf("Clean(nil) must return non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("Clean(nil): want empty, got %v", got)
	}

	got = techtag.Clean([]string{})
	if len(got) != 0 {
		t.Errorf("Clean([]): want empty, got %v", got)
	}
}

// TestClean_TrimsWhitespaceAndDropsEmpty — whitespace-only entries and empty
// strings are dropped; surrounding whitespace is trimmed before normalization.
func TestClean_TrimsWhitespaceAndDropsEmpty(t *testing.T) {
	got := techtag.Clean([]string{"  ", "", "  Go  ", "\t", "\n"})
	if len(got) != 1 || got[0] != "Go" {
		t.Errorf("expected [Go], got %v", got)
	}
}

// TestClean_CanonicalMappingNormalizesVariants — variant spellings collapse to
// their canonical form regardless of input casing.
func TestClean_CanonicalMappingNormalizesVariants(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"reactjs", "React"},
		{"REACT.JS", "React"},
		{"React", "React"},
		{"k8s", "Kubernetes"},
		{"K8S", "Kubernetes"},
		{"golang", "Go"},
		{"GoLang", "Go"},
		{"NODE.js", "Node.js"},
		{"ruby on rails", "Rails"},
		{"PostgreSQL", "PostgreSQL"},
		{"postgres", "PostgreSQL"},
		{"py", "Python"},
		{"DOTNET", ".NET"},
		{".net", ".NET"},
		{"google cloud", "GCP"},
		{"Amazon Web Services", "AWS"},
		{"Elastic Search", "Elasticsearch"},
		{"dynamo db", "DynamoDB"},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			got := techtag.Clean([]string{c.raw})
			if len(got) != 1 || got[0] != c.want {
				t.Errorf("Clean(%q) = %v, want [%q]", c.raw, got, c.want)
			}
		})
	}
}

// TestClean_BlocklistDropsNoise — blocklist terms are dropped entirely
// regardless of input casing. Methodologies, tools, soft skills, and domain
// terms are no longer blocked (see TestClean_AllowsBroadenedCategories) —
// only genuine noise remains here.
func TestClean_BlocklistDropsNoise(t *testing.T) {
	noise := []string{
		"microsoft office", "excel", "word", "powerpoint",
		"windows", "macos", "unix",
		"api", "apis", "sdk", "json", "xml", "html", "css", "sql",
		"nosql", "version control",
	}
	got := techtag.Clean(noise)
	if len(got) != 0 {
		t.Errorf("expected all noise filtered, got %v", got)
	}
}

// TestClean_AllowsBroadenedCategories — methodologies, non-dev tools/platforms,
// soft skills, and certifications/domain terms now survive Clean, with
// canonical normalization applied where a mapping exists.
func TestClean_AllowsBroadenedCategories(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"agile", "Agile"},
		{"Scrum", "Scrum"},
		{"ci/cd", "CI/CD"},
		{"ci cd", "CI/CD"},
		{"microservices", "Microservices"},
		{"devops", "DevOps"},
		{"leadership", "Leadership"},
		{"communication", "Communication"},
		{"jira", "Jira"},
		{"figma", "Figma"},
		{"salesforce", "Salesforce"},
		{"aws certified", "AWS Certified"},
		{"pmp", "PMP"},
		{"hipaa", "HIPAA"},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			got := techtag.Clean([]string{c.raw})
			if len(got) != 1 || got[0] != c.want {
				t.Errorf("Clean(%q) = %v, want [%q]", c.raw, got, c.want)
			}
		})
	}
}

// TestClean_DeduplicatesCaseInsensitive — duplicates that resolve to the same
// canonical form are collapsed; first occurrence wins.
func TestClean_DeduplicatesCaseInsensitive(t *testing.T) {
	got := techtag.Clean([]string{"React", "reactjs", "REACT", "react.js"})
	if len(got) != 1 || got[0] != "React" {
		t.Errorf("expected [React] after dedup, got %v", got)
	}
}

// TestClean_PreservesOrderOfFirstOccurrence — the slice maintains input order
// of the first seen variant of each canonical tech.
func TestClean_PreservesOrderOfFirstOccurrence(t *testing.T) {
	got := techtag.Clean([]string{"Python", "Go", "Python", "React"})
	want := []string{"Python", "Go", "React"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pos %d: got %q want %q", i, got[i], want[i])
		}
	}
}

// TestClean_UnmappedTermKeepsOriginalCasing — strings not in the canonical or
// blocklist maps pass through with their original casing intact.
func TestClean_UnmappedTermKeepsOriginalCasing(t *testing.T) {
	got := techtag.Clean([]string{"SomeProprietaryFramework", "WeirdTech"})
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %v", got)
	}
	if got[0] != "SomeProprietaryFramework" || got[1] != "WeirdTech" {
		t.Errorf("expected original casing preserved, got %v", got)
	}
}

// TestClean_AmbiguousCasingResolvesViaLowercaseLookup — entries that differ
// only by case are deduped under the canonical form. Confirms the lookup is
// case-insensitive.
func TestClean_AmbiguousCasingResolvesViaLowercaseLookup(t *testing.T) {
	got := techtag.Clean([]string{"GoLang", "GOLANG", "golang", "go"})
	if len(got) != 1 || got[0] != "Go" {
		t.Errorf("expected single canonical [Go], got %v", got)
	}
}

// TestClean_MixedNoiseAndValid — realistic broadened enricher output: some
// valid tech, some valid non-tech keywords, some duplicates, some genuine
// noise; output preserves only allowed canonical terms in input order.
func TestClean_MixedNoiseAndValid(t *testing.T) {
	in := []string{
		"Go", "agile", "REST APIs", "Kubernetes", "k8s",
		"communication", "PostgreSQL", "postgres", "microservices",
		"React", "reactjs", "SQL", "GraphQL", "jira",
	}
	got := techtag.Clean(in)
	want := []string{
		"Go", "Agile", "REST APIs", "Kubernetes", "Communication", "PostgreSQL",
		"Microservices", "React", "GraphQL", "Jira",
	}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pos %d: want %q got %q", i, want[i], got[i])
		}
	}
}

// TestClean_PreservesUniqueCaseSensitiveTerms — different unmapped terms that
// share no lowercase form both survive deduplication.
func TestClean_PreservesUniqueCaseSensitiveTerms(t *testing.T) {
	got := techtag.Clean([]string{"Foo", "Bar", "Baz"})
	if len(got) != 3 {
		t.Fatalf("expected 3 distinct items, got %v", got)
	}
	if strings.Join(got, ",") != "Foo,Bar,Baz" {
		t.Errorf("expected Foo,Bar,Baz order; got %v", got)
	}
}
