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

// TestClean_CanonicalizesSpecificCloudServices — specific named services
// extracted per the tech_stack prompt's "prefer specific over generic"
// instruction normalize consistently, distinct from their generic parent.
func TestClean_CanonicalizesSpecificCloudServices(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"aws lambda", "AWS Lambda"},
		{"AWS Lambda", "AWS Lambda"},
		{"amazon lambda", "AWS Lambda"},
		{"azure functions", "Azure Functions"},
		{"google cloud functions", "Google Cloud Functions"},
		{"ec2", "EC2"},
		{"amazon ec2", "EC2"},
		{"s3", "S3"},
		{"eks", "EKS"},
		{"rds", "RDS"},
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

// TestClean_ReconcilesDescriptivePhrasingToCanonicalForm — if the LLM still
// emits a descriptive variant like "CI/CD pipelines" despite the prompt's
// canonical-naming instruction, Clean normalizes it to the same "CI/CD" used
// for the bare form, preventing fragmented tech_tags for the same concept.
func TestClean_ReconcilesDescriptivePhrasingToCanonicalForm(t *testing.T) {
	got := techtag.Clean([]string{"CI/CD pipelines", "ci cd pipelines"})
	want := []string{"CI/CD"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("Clean(CI/CD pipelines variants) = %v, want %v", got, want)
	}
}

// TestClean_BlocklistDropsGenericDegreeMentions — fieldless degree mentions
// are noise regardless of the tech_stack prompt's degree carve-out, since an
// LLM slip that emits a bare degree would otherwise appear in nearly every
// job and dilute the resume-match signal.
func TestClean_BlocklistDropsGenericDegreeMentions(t *testing.T) {
	noise := []string{
		"Bachelor's degree", "bachelors degree", "bachelor degree",
		"Master's degree", "masters degree", "master degree",
		"college degree", "undergraduate degree", "degree required",
	}
	got := techtag.Clean(noise)
	if len(got) != 0 {
		t.Errorf("expected all generic degree mentions filtered, got %v", got)
	}
}

// TestClean_AllowsFieldSpecificDegree — a degree mention naming a specific
// field of study is not blocklisted (only the bare/fieldless form is noise).
func TestClean_AllowsFieldSpecificDegree(t *testing.T) {
	got := techtag.Clean([]string{"Bachelor's in Computer Science"})
	want := []string{"Bachelor's in Computer Science"}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("Clean(field-specific degree) = %v, want %v", got, want)
	}
}

// TestClean_CollapsesInternalWhitespace — multi-word phrases with irregular
// internal whitespace (double spaces, tabs) normalize to single-spaced form,
// so the frontend's literal-substring regex matcher has one exact string to
// search for regardless of how the LLM spaced the phrase.
func TestClean_CollapsesInternalWhitespace(t *testing.T) {
	got := techtag.Clean([]string{"AWS   Lambda", "Some\tProprietary  Framework"})
	want := []string{"AWS Lambda", "Some Proprietary Framework"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pos %d: got %q want %q", i, got[i], want[i])
		}
	}
}

// TestClean_DoesNotSuppressRedundantSpecificAndGenericPair — documents current
// behavior: Clean has no cross-item awareness, so if the LLM emits both a
// generic parent and its specific child for the same job (e.g. "AWS" and "AWS
// Lambda"), both survive as distinct entries. Avoiding this pair is the
// prompt's responsibility (see Enrich.go's "instead of — not in addition to"
// instruction); Clean only canonicalizes and dedupes exact-match strings.
func TestClean_DoesNotSuppressRedundantSpecificAndGenericPair(t *testing.T) {
	got := techtag.Clean([]string{"AWS", "AWS Lambda"})
	want := []string{"AWS", "AWS Lambda"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pos %d: got %q want %q", i, got[i], want[i])
		}
	}
}
