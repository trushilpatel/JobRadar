package prompts_test

import (
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/service/llm/prompts"
)

func TestBuildResumeKeywords_EmbedsResumeText(t *testing.T) {
	prompt := prompts.BuildResumeKeywords("Experienced with Go, PostgreSQL, and Kubernetes.")
	if !strings.Contains(prompt, "Experienced with Go, PostgreSQL, and Kubernetes.") {
		t.Fatal("expected resume text embedded verbatim into the prompt")
	}
}

func TestBuildResumeKeywords_NoMarkdownInstruction(t *testing.T) {
	prompt := prompts.BuildResumeKeywords("text")
	if !strings.Contains(prompt, "Return ONLY raw JSON") {
		t.Error("ResumeKeywords prompt must instruct raw JSON output")
	}
	if !strings.Contains(prompt, "No markdown formatting") {
		t.Error("ResumeKeywords prompt must instruct the model to skip markdown formatting")
	}
}

func TestBuildResumeKeywords_ExcludesSoftSkillsInstruction(t *testing.T) {
	prompt := prompts.BuildResumeKeywords("text")
	if !strings.Contains(prompt, "Exclude: soft skills") {
		t.Fatal("expected explicit soft-skill exclusion instruction")
	}
}

func TestBuildResumeKeywords_DeterministicForSameInput(t *testing.T) {
	a := prompts.BuildResumeKeywords("Go, PostgreSQL")
	b := prompts.BuildResumeKeywords("Go, PostgreSQL")
	if a != b {
		t.Fatal("BuildResumeKeywords must be deterministic given identical inputs")
	}
}

func TestBuildResumeKeywords_EmptyTextStillProducesValidPrompt(t *testing.T) {
	prompt := prompts.BuildResumeKeywords("")
	if len(prompt) == 0 {
		t.Fatal("prompt must not be empty even for empty resume text")
	}
}
