# Gem ATS Adapter

Public, unauthenticated REST adapter for fetching job postings from a Gem-hosted careers board.

## Overview

Gem is a **fat adapter** — the listing endpoint returns the full job description for every posting, so no per-job detail fetch is required. `JobContent` is always populated at listing time. `PostFetcher` is **not** implemented (Gem's per-job endpoint returns 404).

## API

Base URL: `https://api.gem.com`

**Endpoint:** `GET /job_board/v0/{slug}/job_posts/`

- Requires `Accept: application/json` header.
- Returns a JSON array `[]JobPost` (not an object wrapper).
- No authentication required.

## Mapping

| Gem field             | `core.JobSummary` field    | Notes                                                      |
| --------------------- | -------------------------- | ---------------------------------------------------------- |
| `id`                  | `ID`                       | Prefixed: `"gem-" + id`                                    |
| `title`               | `Title`                    |                                                            |
| `absolute_url`        | `URL`                      |                                                            |
| `location.name`       | `Location`                 | Falls back to joined `offices[*].location.name` when empty |
| `employment_type`     | `EmployType`               | Normalized via `mapEmploymentType`                         |
| —                     | `LocationKnown`            | Always `true`                                              |
| —                     | `AllLocations`             | Single-element slice containing the resolved location      |
| `content_plain`       | `JobContent.Description`   | Preferred; falls back to `StripHTML(content)`              |
| `departments[0].name` | `JobContent.Department`    | First entry only                                           |
| `location_type`       | `JobContent.WorkplaceType` | `remote` / `hybrid` / `onsite` / `""`                      |
| `first_published_at`  | `JobContent.PublishedDate` | Falls back to `updated_at` then `created_at`               |

## Employment type mapping

| Gem value    | `core` constant         |
| ------------ | ----------------------- |
| `full_time`  | `core.EmployFullTime`   |
| `part_time`  | `core.EmployPartTime`   |
| `contract`   | `core.EmployContract`   |
| `intern`     | `core.EmployInternship` |
| `internship` | `core.EmployInternship` |
| _(unknown)_  | `core.EmployFullTime`   |

## Workplace type mapping

| Gem value   | Pipeline value |
| ----------- | -------------- |
| `remote`    | `remote`       |
| `hybrid`    | `hybrid`       |
| `in_office` | `onsite`       |
| `onsite`    | `onsite`       |
| `on_site`   | `onsite`       |
| _(unknown)_ | `""`           |

## Integration test

Uses `"inception"` as a known Gem customer. Guard with `testing.Short()` — runs only when `-short` is not set.
