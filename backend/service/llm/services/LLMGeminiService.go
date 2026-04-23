package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/genai"

	llmerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/trushilpatel/jobradar/service/llm/prompts"
)

type geminiScorer struct {
	client      *genai.Client
	modelName   string
	targetRoles []string
	yearsExp    int
}

func newGeminiScorer(ctx context.Context, apiKey, modelName string, targetRoles []string, yearsExp int) (Scorer, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
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

// ── ScoreBatch ────────────────────────────────────────────────────────────────

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

	raw, err := s.generate(ctx, prompt, scoreBatchSchema)
	if err != nil {
		return nil, fmt.Errorf("gemini score batch: %w", err)
	}

	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil, fmt.Errorf("gemini score batch: parse response %q: %w", raw, err)
	}
	return ids, nil
}

// ── EnrichBatch ───────────────────────────────────────────────────────────────

// enrichItemSchema is the per-job result shape returned inside the batch array.
// job_id is required so callers can attribute results back to source inputs.
var enrichItemSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"job_id":             {Type: genai.TypeString},
		"experience_years":   {Type: genai.TypeInteger, Nullable: boolPtr(true)},
		"tech_stack":         {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
		"salary_min":         {Type: genai.TypeInteger, Nullable: boolPtr(true)},
		"salary_max":         {Type: genai.TypeInteger, Nullable: boolPtr(true)},
		"salary_currency":    {Type: genai.TypeString, Nullable: boolPtr(true)},
		"summary":            {Type: genai.TypeString},
		"remote":             {Type: genai.TypeBoolean},
		"country":            {Type: genai.TypeString},
		"state":              {Type: genai.TypeString, Nullable: boolPtr(true)},
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
		"job_id",
		"experience_years", "tech_stack", "salary_min", "salary_max", "salary_currency",
		"summary", "remote", "country", "state", "sponsorship",
		"requires_citizen", "security_clearance", "role_brief", "responsibilities", "role_category",
		"role_match",
	},
}

// enrichBatchSchema wraps enrichItemSchema in an array — the model is instructed
// to return one result object per input job, in the same order, each carrying
// the matching job_id field.
var enrichBatchSchema = &genai.Schema{
	Type:  genai.TypeArray,
	Items: enrichItemSchema,
}

func (s *geminiScorer) EnrichBatch(ctx context.Context, inputs []dto.JobPostEnrichInput) ([]*dto.JobPostEnrichResult, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	prompt := prompts.BuildEnrichBatch(s.targetRoles, inputs)

	raw, err := s.generate(ctx, prompt, enrichBatchSchema)
	if err != nil {
		return nil, fmt.Errorf("gemini enrich batch: %w", err)
	}

	var out []*dto.JobPostEnrichResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("gemini enrich batch: parse response %q: %w", raw, err)
	}

	return out, nil
}

// ── shared generate helper ────────────────────────────────────────────────────

func (s *geminiScorer) generate(ctx context.Context, prompt string, schema *genai.Schema) (string, error) {
	temp := float32(0.0)
	resp, err := s.client.Models.GenerateContent(
		ctx,
		s.modelName,
		[]*genai.Content{genai.NewContentFromText(prompt, genai.RoleUser)},
		&genai.GenerateContentConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   schema,
			Temperature:      &temp,
		},
	)
	if err != nil {
		if isGemini429(err) {
			return "", fmt.Errorf("%w: %s", llmerr.ErrLLMProviderRateLimited, err.Error())
		}
		return "", err
	}

	text := resp.Text()
	if text == "" {
		return "", fmt.Errorf("empty response from model")
	}

	return cleanJSON(text), nil
}

// isGemini429 reports whether err indicates a Gemini provider-side rate limit
// response (HTTP 429). The genai SDK returns a value-typed APIError with the
// raw HTTP status code in Code.
func isGemini429(err error) bool {
	if err == nil {
		return false
	}
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == http.StatusTooManyRequests
	}
	return false
}

func cleanJSON(text string) string {
	text = strings.ReplaceAll(text, "\u00a0", " ")
	text = strings.TrimSpace(text)

	start := strings.IndexAny(text, "{[")
	end := strings.LastIndexAny(text, "}]")

	if start != -1 && end != -1 && end >= start {
		return text[start : end+1]
	}

	return text
}

func boolPtr(b bool) *bool { return &b }
