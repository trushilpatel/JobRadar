# Responsibilities Field: String → Array

**Date:** 2026-04-10  
**Status:** Approved

## Problem

`responsibilities` is currently a newline-separated `TEXT` string produced by the LLM and stored as-is. This is hard to render cleanly in the UI and awkward to work with programmatically.

## Goal

Change `responsibilities` to a structured array of strings throughout the stack — LLM output, Go types, database column, and frontend rendering.

## Scope

All existing data will be truncated (except the `companies` table) before this change is deployed.

## Changes

### 1. Database — new migration `016_responsibilities_jsonb.sql`

```sql
ALTER TABLE job ALTER COLUMN responsibilities TYPE JSONB USING NULL;
```

### 2. `backend/internal/model/job.go`

```go
// before
Responsibilities *string `json:"responsibilities"`

// after
Responsibilities datatypes.JSON `json:"responsibilities"`
```

### 3. `backend/internal/llm/llm.go`

```go
// before
Responsibilities string

// after
Responsibilities []string
```

### 4. `backend/internal/llm/gemini.go`

- Schema: change `responsibilities` from `TypeString` to `TypeArray` with `Items: TypeString` (same pattern as `tech_stack`)
- Prompt: update instruction from "newline-separated" to returning a JSON array of strings

### 5. `backend/internal/llm/anthropic.go`

No prompt or schema changes. One-line fix to map the existing newline-separated string into `[]string` when building `EnrichResult`:

```go
Responsibilities: strings.Split(strings.TrimSpace(out.Responsibilities), "\n"),
```

### 6. `backend/internal/worker/job_post_analyzer/worker.go`

Store `Responsibilities` as `datatypes.JSON` when building the `Job` model, consistent with how `tech_stack` is stored.

### 7. `frontend/src/types/index.ts`

```ts
// before
responsibilities: string | null

// after
responsibilities: string[] | null
```

### 8. `frontend/src/pages/JobsPage.tsx`

Replace `whitespace-pre-line` paragraph with a `<ul>/<li>` list:

```tsx
<ul className="list-disc list-inside space-y-1">
  {job.responsibilities.map((r, i) => (
    <li key={i} className="text-sm text-slate-300">{r}</li>
  ))}
</ul>
```

## What is NOT changing

- `anthropic.go` prompt and schema — Anthropic provider continues to ask for newline-separated text; the split happens in Go when populating `EnrichResult`
- `job_snapshot` table — does not include `responsibilities`, no change needed
