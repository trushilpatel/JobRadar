# internal/llm/ — LLM scoring and enrichment

## Scorer interface

Workers depend on `llm.Scorer` — never on a concrete provider struct.

```go
type Scorer interface {
    ScoreBatch(ctx context.Context, jobs []ScorerInput) (relevantIDs []string, err error)
    Enrich(ctx context.Context, input EnrichInput) (*EnrichResult, error)
}
```

## ScoreBatch vs Enrich

| Method | Input | Purpose | Cost | Max output tokens |
|---|---|---|---|---|
| `ScoreBatch` | title + id only | Filter relevant jobs | Cheap — batch of 50 | 4096 |
| `Enrich` | full page text | Extract structured details | One call per relevant job | 1024 |

`ScoreBatch` uses 4096 output tokens — batch of 50 IDs requires headroom to avoid truncated JSON.

## Adding a new provider

1. Create `{provider}.go` implementing `Scorer`
2. Add a `case` in `factory.go` — that's the only file that changes

```go
func NewScorer(provider, apiKey string, targetRoles []string, yearsExp int) (Scorer, error) {
    switch provider {
    case "anthropic":
        return NewAnthropicScorer(apiKey, targetRoles, yearsExp), nil
    case "openai":                          // ← add here
        return NewOpenAIScorer(apiKey, targetRoles, yearsExp), nil
    }
}
```

## EnrichResult fields

All fields extracted by `Enrich()` and their downstream use:

| Field | Type | Used by |
|---|---|---|
| `ExperienceYears` | `*int` | `job.experience_years`, `job_snapshot.experience_years` |
| `TechStack` | `[]string` | `job.tech_stack`, `job_snapshot.tech_stack` |
| `SalaryMin/Max/Currency` | `*int`, `string` | `job.salary_range` (JSONB) |
| `Summary` | `string` | `job.llm_summary` |
| `Country` | `string` | `job.country`, `job_snapshot.country` — e.g. `"USA"`, `"Canada"` |
| `State` | `string` | `job.state`, `job_snapshot.state` — US state abbrev e.g. `"CA"` |
| `Sponsorship` | `bool` | `job.sponsorship`, `job_snapshot.sponsorship` |
| `RequiresCitizen` | `bool` | `job.requires_citizen`, `job_snapshot.requires_citizen` |
| `SecurityClearance` | `bool` | `job.security_clearance`, `job_snapshot.security_clearance` |
| `RoleBrief` | `string` | `job.role_brief` |
| `Responsibilities` | `string` | `job.responsibilities` |
| `RoleCategory` | `string` | `job_snapshot.role_category` — one of: `backend`, `frontend`, `fullstack`, `devops`, `ml`, `data`, `other` |

When adding a new field: update `EnrichResult`, both provider prompts (`anthropic.go` + `gemini.go`), `buildJob` in the enricher, and `SnapshotInput` if it belongs in analytics.

## Prompt design rule

- **Fixed** (hardcoded): output format, exclusion rules, response structure
- **Dynamic** (from config): `TARGET_ROLES`, `YEARS_EXP` — loaded via `config.Config`

Never hardcode role titles or experience levels in the prompt strings.
