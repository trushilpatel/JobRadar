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

| Method | Input | Purpose | Cost |
|---|---|---|---|
| `ScoreBatch` | title + id only | Filter relevant jobs | Cheap — batch of 50 |
| `Enrich` | full page text | Extract structured details | One call per relevant job |

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

## Prompt design rule

- **Fixed** (hardcoded): output format, exclusion rules, response structure
- **Dynamic** (from config): `TARGET_ROLES`, `YEARS_EXP` — loaded via `config.Config`

Never hardcode role titles or experience levels in the prompt strings.
