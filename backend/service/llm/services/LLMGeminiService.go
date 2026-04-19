package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/trushilpatel/jobradar/service/llm/prompts"
	llmcommon "github.com/trushilpatel/jobradar/service/llm/common"
	"google.golang.org/api/option"
)

type geminiScorer struct {
	client      *genai.Client
	modelName   string
	targetRoles []string
	yearsExp    int
}

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

func (s *geminiScorer) ScoreBatch(ctx context.Context, jobs []dto.JobsRelevanceFilterInput) ([]string, error) {
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

	prompt := prompts.BuildScoreBatch(s.targetRoles, s.yearsExp, sb.String())

	raw, err := s.generate(ctx, prompt, llmcommon.ScoreBatchMaxOutputTokens, scoreBatchSchema)
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
		"role_match": {Type: genai.TypeBoolean},
	},
	Required: []string{
		"experience_years", "tech_stack", "salary_min", "salary_max", "salary_currency",
		"summary", "remote", "country", "state", "sponsorship",
		"requires_citizen", "security_clearance", "role_brief", "responsibilities", "role_category",
		"role_match",
	},
}

func (s *geminiScorer) Enrich(ctx context.Context, input dto.JobPostEnrichInput) (*dto.JobPostEnrichResult, error) {
	prompt := prompts.BuildEnrich(input.ID, s.targetRoles, input.RawText)

	raw, err := s.generate(ctx, prompt, llmcommon.EnrichMaxOutputTokens, enrichSchema)
	if err != nil {
		return nil, fmt.Errorf("gemini enrich: %w", err)
	}

	var out dto.JobPostEnrichResult
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

	return s.cleanJSON(fullText.String()), nil
}

func (s *geminiScorer) cleanJSON(text string) string {
	text = strings.ReplaceAll(text, "\u00a0", " ")
	text = strings.TrimSpace(text)

	start := strings.IndexAny(text, "{[")
	end := strings.LastIndexAny(text, "}]")

	if start != -1 && end != -1 && end >= start {
		return text[start : end+1]
	}

	return text
}
