# crawler-v2

**Status: wired in** — the canonical ATS adapter library for the pipeline. Crawler-node uses it for listing fetches and the analyzer uses it for `FetchJobPost` (single-post detail). The standalone v1 monolith crawler has been removed; this package (formerly `jobingest`) is the only ATS adapter source. Fetches job listings and details from ATS providers and maps them to canonical types consumed by the filter + analyzer pipeline.

## Module path

`github.com/trushilpatel/jobradar/apps/crawler-v2` (part of the main `backend` module — no separate `go.mod`)

## Core interfaces (`core/interfaces.go`)

| Interface                    | Purpose                                                                        |
| ---------------------------- | ------------------------------------------------------------------------------ |
| `ListingFetcher`             | Fetches a company's job listing page                                           |
| `PostFetcher`                | Fetches full details for a single job post                                     |
| `ConfigurableListingFetcher` | Like `ListingFetcher` but accepts per-company JSON config (Workday, Eightfold) |

## Adapter patterns

Three distinct patterns exist across the 14 adapters:

**Two-phase thin adapters** — `FetchJobListing` returns summaries with `JobContent = nil`; `FetchJobPost` fetches detail on demand. Implements both `ListingFetcher` and `PostFetcher`.
→ Lever, Ashby, Netflix, Microsoft, Rippling, Apple, Meta, Google

**Fat adapters** — `FetchJobListing` returns summaries with `JobContent` fully populated (description, department, etc.); no `PostFetcher`.
→ Gem, Uber, Deel

**Configurable adapters** — per-company JSON config required (host, site, domain). `FetchJobListing` always errors with "requires config; use FetchJobListingWithConfig". Implements `ListingFetcher` (stub), `ConfigurableListingFetcher`, and `PostFetcher`.
→ Workday, Eightfold

## Adapter folder structure

Each adapter package follows this layout:

```
adapters/ats/{name}/
├── api/types.go           # raw JSON response structs — no core imports
├── {name}.go              # struct + HTTP methods + interface checks
├── mapper.go              # pure mapping functions
├── {name}_test.go         # unit tests via httptest.NewServer
├── mapper_test.go         # mapper unit tests
├── {name}_integration_test.go  # guarded by testing.Short()
├── testdata/              # JSON/HTML/XML fixtures
└── {name}.md              # API knowledge doc
```

## Key rules

- `StripHTML` is in `adapters/ats/html.go` (package `ats`) — import as `ats.StripHTML(s)` in all adapter sub-packages
- Employment type constants in `core/constants.go` — use `core.EmployFullTime` etc., never raw strings
- All IDs follow `{adapter}-{platformID}` format (e.g. `"lever-abc-123"`, `"workday-REQ001"`)
- `Description` fields in `core.JobPost` are **plain text** — always call `ats.StripHTML` before storing HTML content
- All errors wrapped with `fmt.Errorf("adapter: context: %w", err)` at each layer
- Logging via `logger.Logger` only — never stdlib `log` or `fmt.Println`
- Integration tests **must** be guarded with `if testing.Short() { t.Skip(...) }` — they hit real APIs
- Compile-time interface checks required in every adapter's `.go` file

## Coverage gate

Per-package coverage ≥95% (same as backend gate). Run:

```bash
go test -short -coverprofile=/tmp/cov.out ./...
go tool cover -func=/tmp/cov.out
```
