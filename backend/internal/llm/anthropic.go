package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type anthropicScorer struct {
	client      *anthropic.Client
	modelName   string
	targetRoles []string
	yearsExp    int
}

// newAnthropicScorer creates an Anthropic-backed Scorer.
// modelName, targetRoles, and yearsExp come from ScorerOptions — never hardcoded.
func newAnthropicScorer(apiKey, modelName string, targetRoles []string, yearsExp int) Scorer {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &anthropicScorer{
		client:      &client,
		modelName:   modelName,
		targetRoles: targetRoles,
		yearsExp:    yearsExp,
	}
}

// --- ScoreBatch ---

// ScoreBatch sends job titles to Claude and asks which ones are relevant
// for a software engineering role. Returns only the IDs of relevant jobs.
//
// We send title + id only (not full job text) — this keeps the prompt
// small and cheap. Full text extraction happens later in Enrich().
func (s *anthropicScorer) ScoreBatch(ctx context.Context, jobs []ScorerInput) ([]string, error) {
	if len(jobs) == 0 {
		return nil, nil
	}

	// Build a numbered list of job titles for the prompt
	var sb strings.Builder
	for i, j := range jobs {
		fmt.Fprintf(&sb, "%d. [%s] %s\n", i+1, j.ID, j.Title)
	}

	roles := strings.Join(s.targetRoles, ", ")
	prompt := fmt.Sprintf(`You are filtering job listings for a candidate with the following preferences:
- Target roles: %s
- Minimum years of experience: %d

Here is a list of job titles with their IDs:
%s
Return ONLY a JSON array of the IDs that are a strong match for the target roles above.
Exclude: internships, management-only roles, sales, marketing, design, and any non-engineering roles.
Exclude roles requiring significantly more experience than specified.

Respond with ONLY the JSON array, no explanation. Example: ["greenhouse-123", "lever-456"]`,
		roles, s.yearsExp, sb.String(),
	)

	resp, err := s.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(s.modelName),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("anthropic score batch: %w", err)
	}

	raw := resp.Content[0].Text
	raw = strings.TrimSpace(raw)

	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil, fmt.Errorf("anthropic score batch: parse response %q: %w", raw, err)
	}

	return ids, nil
}

// --- Enrich ---

// Enrich sends the full job page text to Claude and extracts structured details.
// This is called once per relevant job, so quality matters more than speed here.
func (s *anthropicScorer) Enrich(ctx context.Context, input EnrichInput) (*EnrichResult, error) {
	prompt := fmt.Sprintf(`Extract structured information from the following job posting.

Job ID: %s

Job posting text:
%s

Return a JSON object with exactly these fields (use null for missing fields):
{
  "experience_years": <minimum years of experience required as integer, or null>,
  "tech_stack": <array of key tools/languages/frameworks mentioned>,
  "salary_min": <integer or null>,
  "salary_max": <integer or null>,
  "salary_currency": <string or null>,
  "summary": "<2-3 sentence summary of the role>",
  "sponsorship": <true if the posting mentions H1B/visa sponsorship support, false otherwise>,
  "requires_citizen": <true if the posting requires US Citizen or Green Card only, false otherwise>,
  "security_clearance": <true if any security clearance (Secret, TS/SCI, etc.) is required, false otherwise>,
  "role_brief": "<one sentence technical mission of the role>",
  "responsibilities": "<concise bullet points of key technical responsibilities, newline-separated>"
}

Respond with ONLY the JSON object, no explanation.`,
		input.ID, input.RawText,
	)

	resp, err := s.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(s.modelName),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("anthropic enrich: %w", err)
	}

	raw := strings.TrimSpace(resp.Content[0].Text)

	var out struct {
		ExperienceYears   *int     `json:"experience_years"`
		TechStack         []string `json:"tech_stack"`
		SalaryMin         *int     `json:"salary_min"`
		SalaryMax         *int     `json:"salary_max"`
		SalaryCurrency    string   `json:"salary_currency"`
		Summary           string   `json:"summary"`
		Sponsorship       bool     `json:"sponsorship"`
		RequiresCitizen   bool     `json:"requires_citizen"`
		SecurityClearance bool     `json:"security_clearance"`
		RoleBrief         string   `json:"role_brief"`
		Responsibilities  string   `json:"responsibilities"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("anthropic enrich: parse response %q: %w", raw, err)
	}

	return &EnrichResult{
		JobID:             input.ID,
		ExperienceYears:   out.ExperienceYears,
		TechStack:         out.TechStack,
		SalaryMin:         out.SalaryMin,
		SalaryMax:         out.SalaryMax,
		SalaryCurrency:    out.SalaryCurrency,
		Summary:           out.Summary,
		Sponsorship:       out.Sponsorship,
		RequiresCitizen:   out.RequiresCitizen,
		SecurityClearance: out.SecurityClearance,
		RoleBrief:         out.RoleBrief,
		Responsibilities:  out.Responsibilities,
	}, nil
}
