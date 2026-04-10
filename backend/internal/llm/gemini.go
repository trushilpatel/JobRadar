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

var scoreBatchSchema = &genai.Schema{
	Type:  genai.TypeArray,
	Items: &genai.Schema{Type: genai.TypeString},
}

func (s *geminiScorer) ScoreBatch(ctx context.Context, jobs []ScorerInput) ([]string, error) {
	if len(jobs) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	for i, j := range jobs {
		fmt.Fprintf(&sb, "%d. [%s] %s\n", i+1, j.ID, j.Title)
	}

	roles := strings.Join(s.targetRoles, ", ")
	// Restoration of your original prompt with mandatory JSON guardrails at the top
	prompt := fmt.Sprintf(`Return ONLY a valid JSON array of strings. DO NOT use markdown. NO preamble.

You are a job relevance filter for a software engineer with the following profile:
- Target roles: %s
- Years of experience: %d+ years

Evaluate each job title and location to return the IDs of jobs that are a plausible match.

SENIORITY GUIDELINES
INCLUDE: Junior, Mid-level, Senior, Software Engineer I/II/III, SWE, Backend, Frontend, Fullstack, Platform, Infrastructure, DevOps, SRE, Data Engineer, ML Engineer, and similar IC (Individual Contributor) roles.
INCLUDE: Ambiguous or unclear seniority - prefer false positives over false negatives.
EXCLUDE: Staff, Principal, Distinguished, Fellow, Architect (standalone), Director, VP, C-suite, Head of Engineering, or any role that is explicitly a people manager with no IC track.
EXCLUDE: Internships, Co-ops, Apprenticeships.
EXCLUDE: Non-engineering roles (Sales, Marketing, Design, Recruiting, Finance, Legal).

LOCATION GUIDELINES
INCLUDE: Positions explicitly located within the USA.
INCLUDE: Positions listed as Remote (if the company is US-based or specifies US-remote).
EXCLUDE: Any positions located outside of the USA (e.g., EMEA, APAC, Canada, LATAM) or remote roles restricted to non-US time zones.

When in doubt, INCLUDE the job. A borderline match included here can be filtered later. A relevant job excluded here is lost permanently.

Return a JSON array of matching job IDs. Return an empty array if none match.

Job list:
%s`,
		roles, s.yearsExp, sb.String(),
	)

	raw, err := s.generate(ctx, prompt, 4096, scoreBatchSchema)
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

var enrichSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"experience_years":   {Type: genai.TypeInteger, Nullable: true},
		"tech_stack":         {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
		"salary_min":         {Type: genai.TypeInteger, Nullable: true},
		"salary_max":         {Type: genai.TypeInteger, Nullable: true},
		"salary_currency":    {Type: genai.TypeString, Nullable: true},
		"summary":            {Type: genai.TypeString},
		"remote":             {Type: genai.TypeBoolean},
		"country":            {Type: genai.TypeString},
		"state":              {Type: genai.TypeString, Nullable: true},
		"sponsorship":        {Type: genai.TypeBoolean},
		"requires_citizen":   {Type: genai.TypeBoolean},
		"security_clearance": {Type: genai.TypeBoolean},
		"role_brief":         {Type: genai.TypeString},
		"responsibilities":   {Type: genai.TypeString},
		"role_category": {
			Type: genai.TypeString,
			Enum: []string{"backend", "frontend", "fullstack", "devops", "ml", "data", "other"},
		},
	},
	Required: []string{
		"tech_stack", "summary", "remote", "country", "sponsorship",
		"requires_citizen", "security_clearance", "role_brief", "responsibilities", "role_category",
	},
}

func (s *geminiScorer) Enrich(ctx context.Context, input EnrichInput) (*EnrichResult, error) {
	// Restoration of your second prompt with mandatory JSON guardrails at the top
	prompt := fmt.Sprintf(`Return ONLY raw JSON. No markdown formatting.

Extract structured information from the following job posting. Strictly evaluate the text provided without using external knowledge or making assumptions about company policies not explicitly stated in the content.

Job ID: %s

Job posting text:
%s

Field instructions:
- experience_years: minimum years of experience required as integer, or null if not stated
- tech_stack: array of key tools, languages, and frameworks explicitly mentioned
- salary_min, salary_max: integers or null if not stated
- salary_currency: currency code (e.g. "USD") or null
- summary: 2-3 sentence summary of the role
- remote: true if remote or hybrid, false if fully on-site
- country: country where candidate must be based (e.g. "USA", "Canada"). Empty string if no restriction.
- state: US state abbreviation if restricted (e.g. "CA"), null otherwise
- sponsorship: false ONLY if posting explicitly denies H-1B/visa support or requires US citizenship/clearance; true otherwise
- requires_citizen: true ONLY if posting explicitly requires US Citizen or Green Card
- security_clearance: true if any clearance (Secret, TS/SCI, etc.) is required
- role_brief: one sentence technical mission of the role
- responsibilities: concise bullet points of key technical responsibilities, newline-separated
- role_category: one of backend, frontend, fullstack, devops, ml, data, other`,
		input.ID, input.RawText,
	)

	raw, err := s.generate(ctx, prompt, 1024, enrichSchema)
	if err != nil {
		return nil, fmt.Errorf("gemini enrich: %w", err)
	}

	var out EnrichResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("gemini enrich: parse response %q: %w", raw, err)
	}

	return &out, nil
}

// ── shared generate helper ───────────────────────────────────────────────────

func (s *geminiScorer) generate(ctx context.Context, prompt string, maxOutputTokens int32, schema *genai.Schema) (string, error) {
	model := s.client.GenerativeModel(s.modelName)
	model.GenerationConfig.MaxOutputTokens = &maxOutputTokens
	model.GenerationConfig.ResponseMIMEType = "application/json"
	model.GenerationConfig.ResponseSchema = schema

	temp := float32(0.0)
	model.GenerationConfig.Temperature = &temp

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", err
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return "", fmt.Errorf("empty response from model")
	}

	var fullText strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if t, ok := part.(genai.Text); ok {
			fullText.WriteString(string(t))
		}
	}

	// Safety layer to strip markdown or conversational text if it appears
	return s.cleanJSON(fullText.String()), nil
}

func (s *geminiScorer) cleanJSON(text string) string {
	text = strings.ReplaceAll(text, "\u00a0", " ")
	text = strings.TrimSpace(text)

	// Finds the actual JSON boundaries to ignore any markdown blocks or text around it
	start := strings.IndexAny(text, "{[")
	end := strings.LastIndexAny(text, "}]")

	if start != -1 && end != -1 && end >= start {
		return text[start : end+1]
	}

	return text
}
