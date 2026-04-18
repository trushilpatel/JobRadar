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
		loc := j.Location
		if loc == "" {
			loc = "Unknown"
		}
		fmt.Fprintf(&sb, "%d. [%s] %s | Location: %s\n", i+1, j.ID, j.Title, loc)
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
EXCLUDE: Systems Administrator, IT support, Staff, Principal, Distinguished, Fellow, Architect (standalone), Director, VP, C-suite, Head of Engineering, or any role that is explicitly a people manager with no IC track.
EXCLUDE: Internships, Co-ops, Apprenticeships, part-time.
EXCLUDE: Non-engineering roles (Sales, Marketing, Design, Recruiting, Finance, Legal).

LOCATION GUIDELINES
INCLUDE: Positions with a US city, US state, or "United States" in the location.
INCLUDE: Positions listed as "Remote" or "Remote - USA" or similar US-remote variants.
INCLUDE: Positions with no location or "Unknown" location — assume US unless the title signals otherwise.
EXCLUDE: Positions with a non-US city or country in the location (e.g. London, Toronto, Berlin, Sydney, India, Canada).
EXCLUDE: Positions that are explicitly "Remote - [non-US country]" or list only non-US locations.

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
		"responsibilities":   {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
		"role_category": {
			Type: genai.TypeString,
			Enum: []string{"backend", "frontend", "fullstack", "devops", "ml", "data", "other"},
		},
	},
	Required: []string{
		"experience_years", "tech_stack", "salary_min", "salary_max", "salary_currency",
		"summary", "remote", "country", "state", "sponsorship",
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
- experience_years: Extract the minimum years of experience as an integer. Recognize all common phrasings: "3-5 years" → 3, "3+ years" → 3, "at least 3 years" → 3, "minimum 3 years" → 3, "3 to 5 years" → 3, "5 or more years" → 5. Treat "preferred", "ideally", "typically", and "we're looking for" the same as "required" — extract the number regardless. If no explicit number appears, infer from seniority: Entry/Junior/Associate/New Grad → 0, Mid/Mid-Level → 3, Senior/Sr → 5, Staff/Principal → 7. Return null only if no number and no seniority indicator appears anywhere in the posting.
- tech_stack: array of specific programming languages, frameworks, databases, cloud platforms, and developer tools explicitly named in the posting. Include: languages (Go, Python, Java, TypeScript), frameworks (React, Django, Spring Boot), databases (PostgreSQL, Redis, MongoDB), cloud services (AWS, GCP, Azure), and dev tools (Docker, Kubernetes, Terraform, Git). Exclude: methodologies (Agile, Scrum, Kanban), soft skills (Communication, Leadership), architectural patterns (REST, MVC, Microservices, CI/CD), and generic office tools (Excel, Word, Microsoft Office). Use canonical names (e.g. "PostgreSQL" not "Postgres", "React" not "ReactJS", "Node.js" not "Node").
- salary_min, salary_max: integers or null if not stated
- salary_currency: Extract salary or salary range. if not found null.
- summary: 2-3 sentence summary of the role only, do not include any information about company
- remote: true if remote or hybrid, false if fully on-site
- country: country where the candidate must be based. Use "USA" for any US city, US state, or "United States". Use the country name (e.g. "Canada", "UK") if the posting explicitly restricts to a non-US location. Empty string only if the posting has no location restriction at all (worldwide remote, no country mentioned).
- state: US state abbreviation if restricted (e.g. "CA"), null otherwise
- sponsorship: false ONLY if the posting contains explicit language such as "no sponsorship", "US person", "U.S. export controlled", "will not sponsor", "sponsorship not available", "must be authorized to work", "does not offer visa sponsorship", or similar denial phrases. Default true if not mentioned.
- requires_citizen: true ONLY if the posting explicitly states "US Citizen", "US Citizenship required", "must be a US citizen", "Green Card required", or "must have permanent residency". Default false if not mentioned.
- security_clearance: true if the posting mentions any security clearance in any form — required, preferred, or eligibility to obtain. This includes: "US Person", "Secret", "Top Secret", "TS/SCI", "clearance required", "must obtain clearance", "eligible to obtain clearance", "ability to obtain", "active clearance", "U.S. security clearance", or any mention of clearance levels. When in doubt, set true. Default false ONLY if the word "clearance" does not appear at all.
- role_brief: one sentence technical mission of the role
- responsibilities: array of concise strings, one key technical responsibility per element, no noise
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
	model.MaxOutputTokens = &maxOutputTokens
	model.ResponseMIMEType = "application/json"
	model.ResponseSchema = schema

	temp := float32(0.0)
	model.Temperature = &temp

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
