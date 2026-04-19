# pipeline/techtag/ — Tech stack normalization

Normalizes and deduplicates raw `tech_stack` strings extracted by the LLM before they are written to the database and `job_tech_tags` table.

## API

```go
// Clean normalizes a raw tech_stack slice from LLM extraction.
// Returns deduplicated canonical tech names with noise removed.
func Clean(raw []string) []string
```

Called by the `job_post_analyzer` worker after enrichment, before writing to the DB.

## What Clean does

1. **Trims whitespace** — drops empty strings
2. **Blocklist** — drops methodologies, soft skills, patterns, and generic tools (e.g. `agile`, `rest`, `sql`, `html`, `ci/cd`)
3. **Canonical mapping** — normalizes variant spellings to a canonical name (e.g. `reactjs` → `React`, `golang` → `Go`, `k8s` → `Kubernetes`)
4. **Deduplication** — case-insensitive; keeps first occurrence

## Extending

- To add a new canonical mapping: add `"variant": "Canonical"` to the `canonical` map
- To block a new noise term: add it (lowercase) to the `blocklist` map
- Both maps use lowercase keys — input is lowercased before lookup
