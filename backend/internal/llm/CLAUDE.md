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
    case "gemini":
        return newGeminiScorer(ctx, apiKey, model, targetRoles, yearsExp)
    case "openai":                          // ← add here
        return NewOpenAIScorer(apiKey, targetRoles, yearsExp), nil
    }
}
```

Currently only `gemini` is supported. `gemini.go` uses structured JSON schema (`ResponseSchema`) for reliable output — new providers should do the same where possible.

## EnrichResult fields

All fields extracted by `Enrich()` and their downstream use:

| Field | Type | Used by |
|---|---|---|
| `ExperienceYears` | `*int` | `job_feed.experience_years`, `job_analytics.experience_years` |
| `TechStack` | `[]string` | `job_feed.tech_stack`, `job_analytics.tech_stack` |
| `SalaryMin/Max/Currency` | `*int`, `string` | `job_feed.salary_range` (JSONB) |
| `Summary` | `string` | `job_feed.llm_summary` |
| `Country` | `string` | `job_feed.country`, `job_analytics.country` — e.g. `"USA"`, `"Canada"` |
| `State` | `string` | `job_feed.state`, `job_analytics.state` — US state abbrev e.g. `"CA"` |
| `Sponsorship` | `bool` | `job_feed.sponsorship`, `job_analytics.sponsorship` |
| `RequiresCitizen` | `bool` | `job_feed.requires_citizen`, `job_analytics.requires_citizen` |
| `SecurityClearance` | `bool` | `job_feed.security_clearance`, `job_analytics.security_clearance` |
| `RoleBrief` | `string` | `job_feed.role_brief` |
| `Responsibilities` | `[]string` | `job_feed.responsibilities` (JSONB array) |
| `RoleCategory` | `string` | `job_analytics.role_category` — one of: `backend`, `frontend`, `fullstack`, `devops`, `ml`, `data`, `other` |

When adding a new field: update `EnrichResult`, `gemini.go` prompt + schema, `buildJob` in the enricher, and `SnapshotInput` if it belongs in analytics.

## Prompt design rule

- **Fixed** (hardcoded): output format, exclusion rules, response structure
- **Dynamic** (from config): `TARGET_ROLES`, `YEARS_EXP` — loaded via `config.Config`

Never hardcode role titles or experience levels in the prompt strings.

## Eligibility filtering

Job dropping happens in the `job_post_analyzer` worker via `dropJob()`, not in the LLM. The LLM's job is accurate extraction — Go enforces the rules:

| Condition | `EnrichResult` field | Drop if |
|---|---|---|
| No sponsorship | `Sponsorship bool` | `false` |
| Citizenship required | `RequiresCitizen bool` | `true` |
| Security clearance required | `SecurityClearance bool` | `true` |
| Over experience limit | `ExperienceYears *int` | `> DISCARD_JOB_WITH_EXP_GT` (0 = disabled) |

The Enrich prompt uses explicit signal phrases for each flag — see `gemini.go`. Default bias: assume sponsorship available, no citizenship requirement, no clearance, unless the posting explicitly says otherwise.
