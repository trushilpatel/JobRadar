package prompts_test

import (
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/service/llm/prompts"
)

func TestBuildScoreBatch_EmbedsRolesYearsAndJobList(t *testing.T) {
	prompt := prompts.BuildScoreBatch(
		[]string{"Backend Engineer", "Software Engineer"},
		5,
		"1. [j1] Senior Software Engineer | Location: New York\n",
	)

	if !strings.Contains(prompt, "Backend Engineer, Software Engineer") {
		t.Fatal("expected roles joined with ', ' embedded into prompt")
	}
	if !strings.Contains(prompt, "5+ years") {
		t.Fatal("expected years of experience embedded")
	}
	if !strings.Contains(prompt, "1. [j1] Senior Software Engineer | Location: New York") {
		t.Fatal("expected job list passed through verbatim")
	}
}

func TestBuildScoreBatch_NoMarkdownInstruction(t *testing.T) {
	prompt := prompts.BuildScoreBatch([]string{"x"}, 0, "")
	if !strings.Contains(prompt, "Return ONLY a valid JSON array of strings") {
		t.Error("ScoreBatch must instruct the model to return a JSON array")
	}
	if !strings.Contains(prompt, "DO NOT use markdown") {
		t.Error("ScoreBatch must instruct the model to skip markdown formatting")
	}
}

func TestBuildScoreBatch_DeterministicForSameInput(t *testing.T) {
	a := prompts.BuildScoreBatch([]string{"Backend", "ML"}, 3, "1. [a] T1 | Location: NY\n")
	b := prompts.BuildScoreBatch([]string{"Backend", "ML"}, 3, "1. [a] T1 | Location: NY\n")
	if a != b {
		t.Fatal("BuildScoreBatch must be deterministic given identical inputs")
	}
}

func TestBuildScoreBatch_RolesOrderPreserved(t *testing.T) {
	// Roles order matters — preserves caller intent and stabilises the prompt
	// for caching/diff reviews.
	a := prompts.BuildScoreBatch([]string{"X", "Y"}, 0, "")
	b := prompts.BuildScoreBatch([]string{"Y", "X"}, 0, "")
	if a == b {
		t.Fatal("role order must alter the prompt text")
	}
}

func TestBuildScoreBatch_EmptyRolesAndYears(t *testing.T) {
	// Boundary: zero years + no roles should still produce a structurally
	// valid prompt, not panic or return empty.
	prompt := prompts.BuildScoreBatch(nil, 0, "")
	if len(prompt) == 0 {
		t.Fatal("prompt must not be empty for zero inputs")
	}
	if !strings.Contains(prompt, "0+ years") {
		t.Fatal("expected '0+ years' for zero experience")
	}
}

func TestBuildScoreBatch_ContainsSeniorityGuidelines(t *testing.T) {
	prompt := prompts.BuildScoreBatch([]string{"Software Engineer"}, 3, "")
	// Guard against silent prompt drift on critical filtering rules.
	if !strings.Contains(prompt, "EXCLUDE: Staff [X], Principal [X]") {
		t.Fatal("ScoreBatch must continue to exclude Staff/Principal IC titles")
	}
	if !strings.Contains(prompt, "EXCLUDE: Internships") {
		t.Fatal("ScoreBatch must exclude internships and co-ops")
	}
	if !strings.Contains(prompt, "Positions with no location or \"Unknown\" location — assume US") {
		t.Fatal("ScoreBatch must keep the 'assume US' rule for missing locations")
	}
}
