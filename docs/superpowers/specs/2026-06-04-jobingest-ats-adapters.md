# jobingest ATS Adapters

**Date:** 2026-06-04  
**Branch:** `worktree-feat+jobingest-ats-adapters`

## What was built

Thirteen ATS adapters added to the `jobingest` module under `jobingest/adapters/ats/`, mirroring the structure of the existing Greenhouse adapter. Each adapter implements one or both core interfaces.

## Shared infrastructure added

- `jobingest/adapters/ats/html.go` — `StripHTML(s string) string` utility shared by all adapter packages
- `jobingest/core/constants.go` — employment type constants (`EmployFullTime`, `EmployPartTime`, `EmployContract`, `EmployInternship`)
- `jobingest/core/interfaces.go` — added `ConfigurableListingFetcher` for adapters requiring per-company JSON config
- `go.work` — added `./jobingest` module entry

## Adapter inventory

| Adapter      | Pattern                  | Interfaces                                                           |
| ------------ | ------------------------ | -------------------------------------------------------------------- |
| `lever/`     | 2-phase REST             | `ListingFetcher`, `PostFetcher`                                      |
| `ashby/`     | 2-phase GraphQL          | `ListingFetcher`, `PostFetcher`                                      |
| `gem/`       | Fat REST                 | `ListingFetcher` only                                                |
| `netflix/`   | 2-phase REST (Eightfold) | `ListingFetcher`, `PostFetcher`                                      |
| `microsoft/` | 2-phase REST             | `ListingFetcher`, `PostFetcher`                                      |
| `rippling/`  | 2-phase Next.js HTML     | `ListingFetcher`, `PostFetcher`                                      |
| `uber/`      | Fat JSON API             | `ListingFetcher` only                                                |
| `deel/`      | Fat RSC stream           | `ListingFetcher` only                                                |
| `apple/`     | 2-phase HTML             | `ListingFetcher`, `PostFetcher`                                      |
| `meta/`      | Sitemap + JSON-LD        | `ListingFetcher`, `PostFetcher`                                      |
| `workday/`   | Configurable 2-phase     | `ListingFetcher` (stub), `ConfigurableListingFetcher`, `PostFetcher` |
| `eightfold/` | Configurable 2-phase     | `ListingFetcher` (stub), `ConfigurableListingFetcher`, `PostFetcher` |
| `google/`    | 2-phase HTML             | `ListingFetcher`, `PostFetcher`                                      |

## Design patterns

- **Fat adapters** (Gem, Uber, Deel): listing response includes full job description; `JobSummary.JobContent` populated at listing time; no `PostFetcher`
- **Two-phase adapters**: listing emits thin summaries (nil `JobContent`); `FetchJobPost` fetches detail on demand
- **Configurable adapters** (Workday, Eightfold): `FetchJobListing` returns error directing callers to `FetchJobListingWithConfig(ctx, companyID, config json.RawMessage)`
- Google is **two-phase** (not fat): description only available at detail page time, not listing time

## Coverage

All packages ≥95% statement coverage. Integration tests in each package are guarded by `testing.Short()`.

## Files per adapter

Each adapter package contains: `api/types.go`, `{name}.go`, `mapper.go`, `{name}_test.go`, `mapper_test.go`, `{name}_integration_test.go`, `testdata/`, `{name}.md`.
