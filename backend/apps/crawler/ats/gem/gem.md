# Gem Job Board API

Public, unauthenticated REST API for fetching job postings from a Gem-hosted careers board.

Base URL:

```
https://api.gem.com
```

---

## 1. List Jobs

**Endpoint:** `GET /job_board/v0/{slug}/job_posts/`

Returns all open job postings for the board in a single response. The listing response already carries the full HTML description — no separate detail fetch is needed. Each `FetchJobsItem` is emitted with `FullContent` populated (full-detail adapter path).

### Request

```bash
curl "https://api.gem.com/job_board/v0/inception/job_posts/"
```

### Response

```json
[
  {
    "id": "abc123",
    "internal_job_id": "INT-456",
    "title": "Software Engineer",
    "absolute_url": "https://jobs.gem.com/inception/posts/abc123",
    "content": "<p>Full HTML job description...</p>",
    "content_plain": "Full plain-text job description...",
    "location": { "name": "San Francisco, CA" },
    "location_type": "hybrid",
    "employment_type": "full_time",
    "departments": [{ "id": "dept-1", "name": "Engineering", "parent_id": null, "child_ids": [] }],
    "offices": [
      {
        "id": "off-1",
        "name": "SF HQ",
        "location": { "name": "San Francisco, CA" },
        "parent_id": null,
        "child_ids": []
      }
    ],
    "requisition_id": "REQ-789",
    "created_at": "2025-01-15T10:00:00Z",
    "updated_at": "2025-03-01T08:30:00Z",
    "first_published_at": "2025-01-16T09:00:00Z"
  }
]
```

### Response Fields

Top-level array — each element is a job posting.

| Field                | Type              | Description                                          | Captured                                                           |
| -------------------- | ----------------- | ---------------------------------------------------- | ------------------------------------------------------------------ |
| `id`                 | string            | Gem posting ID                                       | ✅ → `gem-{id}`                                                    |
| `internal_job_id`    | string            | Internal ATS job ID                                  | —                                                                  |
| `title`              | string            | Job title                                            | ✅                                                                 |
| `absolute_url`       | string            | Public hosted job posting URL                        | ✅                                                                 |
| `content`            | string (HTML)     | Full HTML job description                            | ✅ → stripped to plain text (fallback when `content_plain` absent) |
| `content_plain`      | string            | Pre-stripped plain-text description (preferred)      | ✅ → `FullContent.Description` (takes priority over `content`)     |
| `location.name`      | string            | Primary location                                     | ✅ → `Location` (falls back to office locations)                   |
| `location_type`      | string            | `in_office` \| `remote` \| `hybrid`                  | ✅ → `FullContent.WorkplaceType` (normalized)                      |
| `employment_type`    | string            | `full_time` \| `part_time` \| `contract` \| `intern` | ✅ → `EmploymentType` (normalized)                                 |
| `requisition_id`     | string            | Requisition identifier                               | —                                                                  |
| `created_at`         | string (ISO 8601) | Creation timestamp                                   | ✅ (fallback for `PublishedDate`)                                  |
| `updated_at`         | string (ISO 8601) | Last-updated timestamp                               | ✅ (fallback for `PublishedDate`)                                  |
| `first_published_at` | string (ISO 8601) | First publication timestamp (preferred)              | ✅ → `FullContent.PublishedDate`                                   |

#### `departments[]`

| Field       | Type          | Description          | Captured                                   |
| ----------- | ------------- | -------------------- | ------------------------------------------ |
| `id`        | string        | Department ID        | —                                          |
| `name`      | string        | Department name      | ✅ → `FullContent.Department` (first item) |
| `parent_id` | string / null | Parent department ID | —                                          |
| `child_ids` | string[]      | Child department IDs | —                                          |

#### `offices[]`

| Field                       | Type          | Description            | Captured                                              |
| --------------------------- | ------------- | ---------------------- | ----------------------------------------------------- |
| `id`                        | string        | Office ID              | —                                                     |
| `name`                      | string        | Office name            | —                                                     |
| `location.name`             | string        | Office location string | ✅ (joined as fallback when `location.name` is empty) |
| `parent_id`                 | string / null | Parent office ID       | —                                                     |
| `child_ids`                 | string[]      | Child office IDs       | —                                                     |
| `parent_office_external_id` | string / null | External parent ID     | —                                                     |
| `child_office_external_ids` | string[]      | External child IDs     | —                                                     |

---

## 2. URL Formats

| Usage              | Format                                               |
| ------------------ | ---------------------------------------------------- |
| `companies.url`    | `inception` (board slug only — no URL prefix)        |
| `RawJob.URL` (web) | `https://jobs.gem.com/{slug}/posts/{id}`             |
| Listing API        | `https://api.gem.com/job_board/v0/{slug}/job_posts/` |
| Careers page       | `https://jobs.gem.com/{slug}`                        |

## 3. Adapter notes

- **Full-detail adapter**: `FullContent` is always populated at scan time; the analyzer skips the per-job HTTP fetch.
- No `ContentFetcher` implemented — Gem does not expose a per-job detail endpoint (GET `.../job_posts/{id}` returns 404).
- `PublishedDate` resolution order: `first_published_at` → `updated_at` → `created_at`.
- `location.name` falls back to joined office location names when empty.
