# Deel ATS Adapter

Deel's hosted careers product (`jobs.deel.com`) renders every job board with Next.js Server Components. There is no public JSON REST API — job data is embedded in the React Server Components (RSC) stream.

The adapter requests the RSC stream by setting the `RSC: 1` header, then extracts job records using balanced-brace parsing of the numbered "T" (text) chunks. All content is captured at listing time — no per-job detail fetch is needed (fat adapter; no `PostFetcher`).

Base URL:

```
https://jobs.deel.com
```

---

## 1. Fetch Job Listings

**Interface:** `core.ListingFetcher` only — no `PostFetcher`

**Method:** `GET /{slug}` with `RSC: 1` and `Accept: text/x-component` headers

Returns the raw RSC stream for the company's job board. Job records are embedded as JSON objects within numbered "T" chunks in the stream.

### Request

```bash
curl -H "RSC: 1" -H "Accept: text/x-component" "https://jobs.deel.com/klarna"
```

### RSC stream format

The response is a stream of numbered chunks:

```
1:T<hexLen>,<payload>
15:T<hexLen>,<html description content>
...
```

Job posting records appear as JSON objects embedded in the T-chunks. Each starts with `{"id":"<UUID>","jobId":"<UUID>",` — this prefix is used by the `jobRecordStart` regex to locate records without scanning the entire stream.

### Record structure

```json
{
  "id": "abc12345-...",
  "jobId": "def67890-...",
  "title": "Software Engineer",
  "richtextDescription": "$1f",
  "job": {
    "jobEmploymentTypes": [{ "employmentType": { "name": "Full-time" } }],
    "jobLocations": [{ "location": { "name": "Stockholm, Sweden" } }],
    "jobDepartments": [{ "department": { "name": "Engineering" } }],
    "jobTeams": [{ "team": { "name": "Payments" } }],
    "currentCompensation": {
      "currencyIsoCode": "USD",
      "minAmount": 80000,
      "maxAmount": 120000
    }
  },
  "jobPostingPublications": [{ "currentState": { "createdAt": "2025-03-01T10:00:00Z" } }]
}
```

---

## 2. Field mapping

| Source field                                       | Mapped to                  | Notes                                           |
| -------------------------------------------------- | -------------------------- | ----------------------------------------------- |
| `id`                                               | `JobSummary.ID`            | Prefixed: `deel-{id}`                           |
| `title`                                            | `JobSummary.Title`         |                                                 |
| `job.jobLocations[*].location.name`                | `JobSummary.Location`      | All entries joined with `"; "`                  |
| constructed                                        | `JobSummary.URL`           | `https://jobs.deel.com/{slug}/job-details/{id}` |
| `job.jobEmploymentTypes[0]`                        | `JobSummary.EmployType`    | Normalized via `normalizeDeelEmployment`        |
| always `true`                                      | `JobSummary.LocationKnown` |                                                 |
| `richtextDescription` + chunks                     | `JobContent.Description`   | Dereferenced from chunk map, then `StripHTML`   |
| `job.jobDepartments[0].department.name`            | `JobContent.Department`    | First entry only                                |
| `jobPostingPublications[0].currentState.createdAt` | `JobContent.PublishedDate` | First entry only                                |

Employment type normalization: `"Full-time"` → `full_time`, `"Part-time"` → `part_time`, `"Contract"` / `"Contractor"` / `"Consultant"` / `"Freelance"` → `contract`, `"Intern"` / `"Internship"` → `internship`, all else → `full_time`.

---

## 3. RSC parsing

`parseChunks` walks the stream scanning for `<hexId>:T<hexLen>,` headers using the `chunkHeader` regex, then records `chunkID → text` in a map.

`extractJobRecords` scans the full body for job-posting objects using the `jobRecordStart` regex, then calls `matchBalanced` to bound each JSON object precisely.

`resolveDescription` dereferences `richtextDescription`: if the value is `"$<hexId>"`, the description HTML is looked up from the chunk map and `StripHTML`-stripped; if inline HTML, it is stripped directly; null/empty returns `""`.

---

## 4. Adapter notes

- **Fat adapter**: `JobContent` is always populated at scan time; no `PostFetcher` is implemented.
- `WorkplaceType` is not present in the Deel RSC structure — not mapped.
- Compensation is formatted as `"<min> – <max> <CURRENCY>"` (e.g. `"80000 – 120000 USD"`).
- The `RSC: 1` request header is mandatory — without it, Next.js returns the full HTML document.
- `companyID` is the board slug (e.g. `"klarna"`), not a UUID.

---

## 5. URL formats

| Usage        | Format                                          |
| ------------ | ----------------------------------------------- |
| `companyID`  | `klarna` (board slug only — no URL prefix)      |
| Job URL      | `https://jobs.deel.com/{slug}/job-details/{id}` |
| Careers page | `https://jobs.deel.com/{slug}`                  |
