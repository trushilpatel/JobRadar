package prompts_test

import (
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/service/llm/prompts"
)

func TestBuildEnrich_SecurityClearanceOnlyOnExplicitRequirement(t *testing.T) {
	prompt := prompts.BuildEnrich("job123", []string{"Software Engineer"}, "some job text")

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
	prompt := prompts.BuildEnrich("job123", []string{"Software Engineer"}, "some job text")

	if !strings.Contains(prompt, "Default true if not mentioned") {
		t.Error("sponsorship must default to true when not mentioned in the posting")
	}
}

func TestBuildEnrich_CitizenshipDefaultsFalse(t *testing.T) {
	prompt := prompts.BuildEnrich("job123", []string{"Software Engineer"}, "some job text")

	if !strings.Contains(prompt, "Default false if not mentioned") {
		t.Error("requires_citizen must default to false when not mentioned in the posting")
	}
}
