# Ashby ATS Adapter

Fetches job listings and detail from Ashby's **public hosted jobs** GraphQL endpoint.
This uses the non-authenticated public API only — no API key required.

## API

| Operation | Endpoint                                                                     |
| --------- | ---------------------------------------------------------------------------- |
| Listing   | `POST https://jobs.ashbyhq.com/api/non-user-graphql?op=ApiJobBoardWithTeams` |
| Detail    | `POST https://jobs.ashbyhq.com/api/non-user-graphql?op=ApiJobPosting`        |

Both endpoints require:

- `Content-Type: application/json`
- `User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 ...`

## Listing request body

```json
{
  "operationName": "ApiJobBoardWithTeams",
  "variables": { "organizationHostedJobsPageName": "{slug}" },
  "query": "query ApiJobBoardWithTeams(...) { jobBoardWithTeams(...) { jobPostings { ... } } }"
}
```

Response path: `data.jobBoardWithTeams.jobPostings[]`

## Detail request body

```json
{
  "operationName": "ApiJobPosting",
  "variables": {
    "organizationHostedJobsPageName": "{company}",
    "jobPostingId": "{uuid}"
  },
  "query": "query ApiJobPosting(...) { jobPosting(...) { ... } }"
}
```

The `{company}` and `{uuid}` are parsed from `summary.URL`, which must be the
Ashby hosted jobs URL: `https://jobs.ashbyhq.com/{company}/{uuid}`

Response path: `data.jobPosting`

## ID format

All job IDs are prefixed: `ashby-{uuid}` (e.g. `ashby-abc-123-def-456`).

## Employment type mapping

| Ashby value  | Core constant           |
| ------------ | ----------------------- |
| `FullTime`   | `core.EmployFullTime`   |
| `PartTime`   | `core.EmployPartTime`   |
| `Contract`   | `core.EmployContract`   |
| `Contractor` | `core.EmployContract`   |
| `Temporary`  | `core.EmployContract`   |
| `Intern`     | `core.EmployInternship` |
| `Internship` | `core.EmployInternship` |
| other        | `""` (unknown)          |

## Location format

`locationName` and `workplaceType` are combined as `"{locationName} ({workplaceType})"`.
When `workplaceType` is empty, only `locationName` is used.

`LocationKnown` is always `true`; `AllLocations` contains the formatted location string.

## Files

| File                        | Purpose                                    |
| --------------------------- | ------------------------------------------ |
| `ashby.go`                  | Adapter struct, HTTP wiring, URL parsing   |
| `mapper.go`                 | Pure mapping functions (API types → core)  |
| `api/types.go`              | GraphQL response structs                   |
| `ashby_test.go`             | Unit tests: HTTP, URL parsing, error paths |
| `mapper_test.go`            | Unit tests: mapping and normalization      |
| `ashby_integration_test.go` | Integration tests (skipped with `-short`)  |
| `testdata/listing.json`     | Fixture: `ApiJobBoardWithTeams` response   |
| `testdata/job_post.json`    | Fixture: `ApiJobPosting` response          |
