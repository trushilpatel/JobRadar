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
	prompt := fmt.Sprintf(`You are filtering job listings for a candidate with the following preferences:
- Target roles: %s
- Minimum years of experience: %d

Here is a list of job titles with their IDs:
%s
Return ONLY a JSON array of the IDs that are a strong match for the target roles above.
Exclude: internships, management-only roles, sales, marketing, design, data science, and any non-engineering roles.
Exclude roles requiring significantly more experience than specified.

Respond with ONLY the JSON array, no explanation. Example: ["greenhouse-123", "lever-456"]`,
		roles, s.yearsExp, sb.String(),
	)

	raw, err := s.generate(ctx, prompt, 512)
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
	prompt := fmt.Sprintf(`Extract structured information from the following job posting.

Job ID: %s

Job posting text:
%s

Return a JSON object with exactly these fields (use null for missing fields):
{
  "years_exp_min": <integer or null>,
  "years_exp_max": <integer or null>,
  "tech_stack": <array of strings>,
  "salary_min": <integer or null>,
  "salary_max": <integer or null>,
  "salary_currency": <string or null>,
  "summary": "<2-3 sentence summary of the role>",
  "other_criteria": <object with any other notable requirements>
}

Respond with ONLY the JSON object, no explanation.`,
		input.ID, input.RawText,
	)

	raw, err := s.generate(ctx, prompt, 1024)
	if err != nil {
		return nil, fmt.Errorf("gemini enrich: %w", err)
	}

	var out struct {
		YearsExpMin    *int              `json:"years_exp_min"`
		YearsExpMax    *int              `json:"years_exp_max"`
		TechStack      []string          `json:"tech_stack"`
		SalaryMin      *int              `json:"salary_min"`
		SalaryMax      *int              `json:"salary_max"`
		SalaryCurrency string            `json:"salary_currency"`
		Summary        string            `json:"summary"`
		OtherCriteria  map[string]string `json:"other_criteria"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("gemini enrich: parse response %q: %w", raw, err)
	}

	return &EnrichResult{
		JobID:          input.ID,
		YearsExpMin:    out.YearsExpMin,
		YearsExpMax:    out.YearsExpMax,
		TechStack:      out.TechStack,
		SalaryMin:      out.SalaryMin,
		SalaryMax:      out.SalaryMax,
		SalaryCurrency: out.SalaryCurrency,
		Summary:        out.Summary,
		OtherCriteria:  out.OtherCriteria,
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
