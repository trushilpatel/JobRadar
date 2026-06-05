# Lever Postings API

Public, unauthenticated API for fetching job postings from a Lever-hosted careers board.

Base URL: `https://api.lever.co`

All requests require `?mode=json` — without it Lever returns HTML.

---

## Endpoints

### List Jobs

`GET /v0/postings/{companyID}?mode=json`

Returns all public job postings as a **JSON array** (not an object).

```bash
curl "https://api.lever.co/v0/postings/netlify?mode=json"
```

### Get Single Job

`GET /v0/postings/{company}/{uuid}?mode=json`

Returns full details for one posting as a **JSON object**.

```bash
curl "https://api.lever.co/v0/postings/netlify/abc-111-def?mode=json"
```

---

## URL Patterns

| Usage            | Format                                                        |
| ---------------- | ------------------------------------------------------------- |
| Listing API      | `https://api.lever.co/v0/postings/{company}?mode=json`        |
| Detail API       | `https://api.lever.co/v0/postings/{company}/{uuid}?mode=json` |
| Hosted jobs page | `https://jobs.lever.co/{company}/{uuid}`                      |

The `hostedUrl` field in list/detail responses uses the `jobs.lever.co` domain.
`FetchJobPost` parses that URL to extract `{company}` and `{uuid}` to build the detail API URL.

---

## Field Mapping

### Listing → core.JobSummary

| API field                 | core.JobSummary field | Notes                                       |
| ------------------------- | --------------------- | ------------------------------------------- |
| `id`                      | `ID`                  | Prefixed: `"lever-" + id`                   |
| `text`                    | `Title`               |                                             |
| `categories.location`     | `Location`            |                                             |
| `hostedUrl`               | `URL`                 |                                             |
| `categories.commitment`   | `EmployType`          | Normalized via `normalizeLeverCommitment()` |
| `categories.allLocations` | `AllLocations`        |                                             |
| (always true)             | `LocationKnown`       | Lever always provides location data         |
| (always nil)              | `JobContent`          | Thin adapter — no content in listing        |

### Detail → core.JobPost

| API field                | core.JobPost field | Notes                                                         |
| ------------------------ | ------------------ | ------------------------------------------------------------- |
| `id`                     | `ID`               | Prefixed: `"lever-" + id`                                     |
| `text`                   | `Title`            |                                                               |
| `categories.location`    | `Location`         |                                                               |
| `hostedUrl`              | `URL`              |                                                               |
| `descriptionPlain`       | `Description`      | Primary; `additionalPlain` appended if non-empty              |
| `lists[]`                | `Requirements`     | Sections joined: `"{text}: {StripHTML(content)}"` with `"\n"` |
| `salaryDescriptionPlain` | `Compensation`     | Falls back to formatted `salaryRange` if empty                |
| `categories.department`  | `Department`       | Falls back to `categories.team` if empty                      |
| `categories.commitment`  | `EmploymentType`   | Normalized via `normalizeLeverCommitment()`                   |
| `workplaceType`          | `WorkplaceType`    | `unspecified` / `on-site` / `remote` / `hybrid`               |
| `createdAt`              | `PublishedDate`    | Unix ms stored as string; `""` when zero                      |

---

## normalizeLeverCommitment Table

| Lever value                          | Normalized constant |
| ------------------------------------ | ------------------- |
| `Full-time`, `Fulltime`, `Full time` | `full_time`         |
| `Part-time`, `Parttime`, `Part time` | `part_time`         |
| `Contract`, `Contractor`             | `contract`          |
| `Internship`, `Intern`               | `internship`        |
| anything else                        | `""` (empty)        |

Matching is case-insensitive.

---

## Key Gotchas

- **`mode=json` is required** on every request. Without it, Lever returns HTML.
- **Listing response is a JSON array**, not a JSON object. This differs from most ATS APIs.
- **Detail response is a JSON object** (single posting).
- `descriptionPlain` contains `openingPlain` + body; `additionalPlain` is the closing/about section — they are separate fields.
- `lists[]` content is unstyled HTML `<li>` items — always strip HTML before use.
- `salaryDescriptionPlain` is the human-readable salary string; `salaryRange` is the structured fallback.
- `createdAt` is a Unix millisecond timestamp (int64), not a formatted date string.
