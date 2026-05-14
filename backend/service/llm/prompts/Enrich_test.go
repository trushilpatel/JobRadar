package prompts_test

import (
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/trushilpatel/jobradar/service/llm/prompts"
)

func TestBuildEnrich_SecurityClearanceOnlyOnExplicitRequirement(t *testing.T) {
	prompt := prompts.BuildEnrichBatch([]string{"Software Engineer"}, []dto.JobPostEnrichInput{{ID: "job123", RawText: "some job text"}})

	// Extract just the security_clearance instruction line.
	start := strings.Index(prompt, "- security_clearance:")
	if start == -1 {
		t.Fatal("security_clearance instruction not found in prompt")
	}
	rest := prompt[start+1:]
	var instruction string
	if before, _, found := strings.Cut(rest, "\n-"); found {
		instruction = prompt[start : start+1+len(before)]
	} else {
		instruction = prompt[start:]
	}

	if strings.Contains(instruction, "When in doubt, set true") {
		t.Error("security_clearance must not default to true when in doubt — only explicit requirements qualify")
	}
	// These phrases must only appear in the default-false examples, not as triggers for true.
	// Verify they appear after "Default false" within the instruction.
	defaultFalseIdx := strings.Index(instruction, "Default false")
	if defaultFalseIdx == -1 {
		t.Error("security_clearance instruction must have a 'Default false' clause")
	}
	triggerSection := instruction[:defaultFalseIdx]
	if strings.Contains(triggerSection, "eligibility to obtain") {
		t.Error("security_clearance must not trigger true on 'eligibility to obtain' — that means the employer sponsors it")
	}
	if strings.Contains(triggerSection, "ability to obtain") {
		t.Error("security_clearance must not trigger true on 'ability to obtain' — that is not a hard requirement")
	}
	if strings.Contains(instruction, "Default false ONLY if the word \"clearance\" does not appear at all") {
		t.Error("security_clearance must not use word-presence as its signal — it must require explicit hard requirement language")
	}
}

func TestBuildEnrich_SponsorshipDefaultsTrue(t *testing.T) {
	prompt := prompts.BuildEnrichBatch([]string{"Software Engineer"}, []dto.JobPostEnrichInput{{ID: "job123", RawText: "some job text"}})

	if !strings.Contains(prompt, "Default true if not mentioned") {
		t.Error("sponsorship must default to true when not mentioned in the posting")
	}
}

func TestBuildEnrich_CitizenshipDefaultsFalse(t *testing.T) {
	prompt := prompts.BuildEnrichBatch([]string{"Software Engineer"}, []dto.JobPostEnrichInput{{ID: "job123", RawText: "some job text"}})

	if !strings.Contains(prompt, "Default false if not mentioned") {
		t.Error("requires_citizen must default to false when not mentioned in the posting")
	}
}

func TestBuildEnrich_RolesJoinedWithComma(t *testing.T) {
	prompt := prompts.BuildEnrichBatch(
		[]string{"Backend Engineer", "Software Engineer"},
		[]dto.JobPostEnrichInput{{ID: "j1", RawText: "x"}},
	)
	if !strings.Contains(prompt, "Backend Engineer, Software Engineer") {
		t.Fatal("expected target roles joined with ', '")
	}
}

func TestBuildEnrich_MultipleJobsSeparatedByDelimiter(t *testing.T) {
	inputs := []dto.JobPostEnrichInput{
		{ID: "a", RawText: "TEXT_A"},
		{ID: "b", RawText: "TEXT_B"},
		{ID: "c", RawText: "TEXT_C"},
	}
	prompt := prompts.BuildEnrichBatch([]string{"Software Engineer"}, inputs)

	if strings.Count(prompt, "\n---\n") != 2 {
		t.Fatalf("expected 2 inter-job delimiters for 3 jobs, got prompt:\n%s", prompt)
	}

	// Each job's ID and raw text must appear once, in order.
	idxA := strings.Index(prompt, "Job ID: a")
	idxB := strings.Index(prompt, "Job ID: b")
	idxC := strings.Index(prompt, "Job ID: c")
	if idxA == -1 || idxB == -1 || idxC == -1 {
		t.Fatal("each job ID must appear in the prompt")
	}
	if idxA >= idxB || idxB >= idxC {
		t.Fatal("job order must be preserved in the prompt")
	}
	for _, raw := range []string{"TEXT_A", "TEXT_B", "TEXT_C"} {
		if !strings.Contains(prompt, raw) {
			t.Fatalf("expected raw text %q to appear in prompt", raw)
		}
	}
}

func TestBuildEnrich_SingleJobHasNoDelimiter(t *testing.T) {
	prompt := prompts.BuildEnrichBatch(
		[]string{"Software Engineer"},
		[]dto.JobPostEnrichInput{{ID: "solo", RawText: "alpha"}},
	)
	if strings.Contains(prompt, "\n---\n") {
		t.Fatal("single-job prompt must not contain the inter-job delimiter")
	}
	if !strings.Contains(prompt, "Job ID: solo") || !strings.Contains(prompt, "alpha") {
		t.Fatal("single-job prompt must still include ID + raw text")
	}
}

func TestBuildEnrich_EmptyInputsProducesNoJobsBlock(t *testing.T) {
	prompt := prompts.BuildEnrichBatch([]string{"Software Engineer"}, nil)
	// The header should still be present, but no Job ID lines.
	if strings.Contains(prompt, "Job ID:") {
		t.Fatal("empty inputs must not produce any Job ID lines")
	}
	if !strings.Contains(prompt, "Return ONLY raw JSON") {
		t.Fatal("template header should still be emitted for empty inputs")
	}
}

func TestBuildEnrich_RoleCategoryAndRoleMatchSchema(t *testing.T) {
	prompt := prompts.BuildEnrichBatch([]string{"Software Engineer"}, []dto.JobPostEnrichInput{{ID: "j1", RawText: "x"}})

	// Schema: role_category enum is a load-bearing contract. The prompt must
	// continue to list the exact six allowed categories + "other".
	for _, cat := range []string{"backend", "frontend", "fullstack", "devops", "ml", "data", "other"} {
		if !strings.Contains(prompt, cat) {
			t.Fatalf("role_category enum must mention %q", cat)
		}
	}
	if !strings.Contains(prompt, "role_match: true if this job is a genuine match for the target roles") {
		t.Fatal("role_match instruction text must be present")
	}
}

func TestBuildEnrich_DeterministicForSameInput(t *testing.T) {
	a := prompts.BuildEnrichBatch([]string{"X"}, []dto.JobPostEnrichInput{{ID: "1", RawText: "t"}})
	b := prompts.BuildEnrichBatch([]string{"X"}, []dto.JobPostEnrichInput{{ID: "1", RawText: "t"}})
	if a != b {
		t.Fatal("BuildEnrichBatch must be deterministic for identical inputs")
	}
}
