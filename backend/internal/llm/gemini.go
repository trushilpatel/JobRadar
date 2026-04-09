package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type geminiScorer struct {
	client      *genai.Client
	modelName   string
	targetRoles []string
	yearsExp    int
}

// newGeminiScorer creates a Gemini-backed Scorer.
// modelName comes from config (LLM_MODEL) — never hardcoded here.
func newGeminiScorer(ctx context.Context, apiKey, modelName string, targetRoles []string, yearsExp int) (Scorer, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("gemini: create client: %w", err)
	}
	return &geminiScorer{
		client:      client,
		modelName:   modelName,
		targetRoles: targetRoles,
		yearsExp:    yearsExp,
	}, nil
}

// ── ScoreBatch ───────────────────────────────────────────────────────────────

func (s *geminiScorer) ScoreBatch(ctx context.Context, jobs []ScorerInput) ([]string, error) {
	if len(jobs) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	for i, j := range jobs {
		fmt.Fprintf(&sb, "%d. [%s] %s\n", i+1, j.ID, j.Title)
	}

	roles := strings.Join(s.targetRoles, ", ")
	prompt := fmt.Sprintf(`You are a job relevance filter for a software engineer with the following profile:
- Target roles: %s
- Years of experience: %d+ years

Evaluate each job title and return the IDs of jobs that are a plausible match.

SENIORITY GUIDELINES:
- INCLUDE: Junior, Mid-level, Senior, Software Engineer I/II/III, SWE, Backend,
  Frontend, Fullstack, Platform, Infrastructure, DevOps, SRE, Data Engineer,
  ML Engineer, and similar IC (Individual Contributor) roles
- INCLUDE if ambiguous or unclear seniority — prefer false positives over false negatives
- EXCLUDE: Staff, Principal, Distinguished, Fellow, Architect (standalone),
  Director, VP, C-suite, Head of Engineering, or any role that is explicitly
  a people manager with no IC track
- EXCLUDE: Internships, Co-ops, Apprenticeships
- EXCLUDE: Non-engineering roles — Sales, Marketing, Design, Recruiting, Finance, Legal

IMPORTANT: When in doubt, INCLUDE the job. A borderline match included here can
be filtered later. A relevant job excluded here is lost permanently.

Job list:
%s

Respond with ONLY a JSON array of matching IDs, no explanation.
Example: ["greenhouse-123", "lever-456"]`,
		roles, s.yearsExp, sb.String(),
	)

	// TODO In future get the reason from LLM and add it to the dismissed jobs to analyse if we are not missing any relevant jobs
	raw, err := s.generate(ctx, prompt, 4096)
	if err != nil {
		return nil, fmt.Errorf("gemini score batch: %w", err)
	}

	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil, fmt.Errorf("gemini score batch: parse response %q: %w", raw, err)
	}
	return ids, nil
}

// ── Enrich ───────────────────────────────────────────────────────────────────

func (s *geminiScorer) Enrich(ctx context.Context, input EnrichInput) (*EnrichResult, error) {
	prompt := fmt.Sprintf(`Extract structured information from the following job posting. **Strictly evaluate the text provided without using external knowledge or making assumptions about company policies not explicitly stated in the content.**

Job ID: %s

Job posting text:
%s

Return a JSON object with exactly these fields (use null for missing fields):
{
"experience\_years": \<minimum years of experience required as integer, or null\>,
"tech\_stack": \<array of key tools/languages/frameworks mentioned\>,
"salary\_min": \<integer or null\>,
"salary\_max": \<integer or null\>,
"salary\_currency": \<string or null\>,
"summary": "\<2-3 sentence summary of the role\>",
"remote": \<true if the role is remote or hybrid, false if fully on-site\>,
"country": "\<location restriction — the country where the candidate must be based, e.g. 'USA', 'Canada', 'UK'. Empty string if no country restriction (globally remote or not specified)\>",
"state": "\<US state abbreviation if restricted to a US state, e.g. 'CA', 'NY', 'TX'. null otherwise\>",
"sponsorship": \<true ONLY if the posting text explicitly mentions H1B, visa sponsorship, or work authorization support; false otherwise\>,
"requires\_citizen": \<true ONLY if the posting text explicitly states US Citizen or Green Card is required; false otherwise\>,
"security\_clearance": \<true if any security clearance (Secret, TS/SCI, etc.) is required, false otherwise\>,
"role\_brief": "\<one sentence technical mission of the role\>",
"responsibilities": "\<concise bullet points of key technical responsibilities, newline-separated\>",
"role\_category": "\<one of: backend, frontend, fullstack, devops, ml, data, other\>"
}

Respond with ONLY the JSON object, no explanation.`,
		input.ID, input.RawText,
	)

	raw, err := s.generate(ctx, prompt, 1024)
	if err != nil {
		return nil, fmt.Errorf("gemini enrich: %w", err)
	}

	var out struct {
		ExperienceYears   *int     `json:"experience_years"`
		TechStack         []string `json:"tech_stack"`
		SalaryMin         *int     `json:"salary_min"`
		SalaryMax         *int     `json:"salary_max"`
		SalaryCurrency    string   `json:"salary_currency"`
		Summary           string   `json:"summary"`
		Remote            bool     `json:"remote"`
		Country           string   `json:"country"`
		State             string   `json:"state"`
		Sponsorship       bool     `json:"sponsorship"`
		RequiresCitizen   bool     `json:"requires_citizen"`
		SecurityClearance bool     `json:"security_clearance"`
		RoleBrief         string   `json:"role_brief"`
		Responsibilities  string   `json:"responsibilities"`
		RoleCategory      string   `json:"role_category"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("gemini enrich: parse response %q: %w", raw, err)
	}

	return &EnrichResult{
		JobID:             input.ID,
		ExperienceYears:   out.ExperienceYears,
		TechStack:         out.TechStack,
		SalaryMin:         out.SalaryMin,
		SalaryMax:         out.SalaryMax,
		SalaryCurrency:    out.SalaryCurrency,
		Summary:           out.Summary,
		Remote:            out.Remote,
		Country:           out.Country,
		State:             out.State,
		Sponsorship:       out.Sponsorship,
		RequiresCitizen:   out.RequiresCitizen,
		SecurityClearance: out.SecurityClearance,
		RoleBrief:         out.RoleBrief,
		Responsibilities:  out.Responsibilities,
		RoleCategory:      out.RoleCategory,
	}, nil
}

// ── shared generate helper ───────────────────────────────────────────────────

func (s *geminiScorer) generate(ctx context.Context, prompt string, maxOutputTokens int32) (string, error) {
	model := s.client.GenerativeModel(s.modelName)
	model.GenerationConfig.MaxOutputTokens = &maxOutputTokens
	// Request JSON output directly — reduces hallucinated formatting
	model.GenerationConfig.ResponseMIMEType = "application/json"

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", err
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return "", fmt.Errorf("empty response from model")
	}

	var text string
	for _, part := range resp.Candidates[0].Content.Parts {
		if t, ok := part.(genai.Text); ok {
			text = strings.TrimSpace(string(t))
			break
		}
	}
	if text == "" {
		return "", fmt.Errorf("no text in response")
	}
	return text, nil
}
