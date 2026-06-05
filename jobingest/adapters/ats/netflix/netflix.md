# Netflix — Eightfold AI Adapter (`jobingest`)

Netflix uses [Eightfold AI](https://eightfold.ai) as their ATS platform, hosted at
`explore.jobs.netflix.net`.

## Package layout

| File                          | Purpose                                                     |
| ----------------------------- | ----------------------------------------------------------- |
| `netflix.go`                  | Adapter struct, `FetchJobListing`, `FetchJobPost`           |
| `mapper.go`                   | Pure conversion functions — API types → core types          |
| `api/types.go`                | Eightfold API response structs (`ListResponse`, `Position`) |
| `netflix_test.go`             | Unit tests for the HTTP adapter (httptest server)           |
| `mapper_test.go`              | Unit tests for the pure mapping functions                   |
| `netflix_integration_test.go` | Integration tests — hit real API; skipped with `-short`     |
| `testdata/listing.json`       | Fixture for listing endpoint (2 positions)                  |
| `testdata/job_post.json`      | Fixture for detail endpoint (1 position with HTML)          |

## API endpoints

### List positions

```
GET https://explore.jobs.netflix.net/api/apply/v2/jobs?domain=netflix.com&hl=en&start={start}&num={num}
```

Paginate by incrementing `start` by `num` until `len(positions) < num`.

### Get position detail

```
GET https://explore.jobs.netflix.net/api/apply/v2/jobs/{id}?domain=netflix.com
```

`{id}` is the raw Eightfold `int64` ID — strip the `netflix-` prefix from `summary.ID`.
The response is flat (no `{"position": {...}}` wrapper).

## Notes

- A `User-Agent` browser header is required on all requests to avoid CloudFront 403s.
- `companyID` passed to `FetchJobListing` is ignored — this adapter always targets Netflix's Eightfold host.
- `EmployType` is always `full_time`; Eightfold does not expose employment type in listings.
- `t_create` / `t_update` are Unix seconds; `unixToRFC3339` converts to RFC 3339 (empty if zero).
- `job_description` is empty in the listing response; only populated by the detail endpoint.
- HTML in `job_description` is stripped by `ats.StripHTML` before being stored in `JobPost.Description`.
