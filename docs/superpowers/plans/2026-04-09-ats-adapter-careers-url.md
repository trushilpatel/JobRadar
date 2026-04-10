# ATS Adapter CareersURL Refactor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `CareersURL(id string) string` to the `Adapter` interface so each adapter owns its own URL construction, then fix the crawler to pass `company.ID` (slug) instead of `company.URL`.

**Architecture:** Each adapter implements `CareersURL` to return the public careers page URL for a given company slug. `FetchJobs` for Greenhouse gains an internal `listURL` helper. Ashby drops `SlugFromURL`. The crawler passes `company.ID` to all three adapters. `company.url` becomes display-only.

**Tech Stack:** Go, goose migrations, React/TypeScript

---

### Task 1: Add `CareersURL` to the `Adapter` interface

**Files:**
- Modify: `backend/internal/ats_fetcher/types/types.go`

- [ ] **Step 1: Add `CareersURL` to the interface**

In `backend/internal/ats_fetcher/types/types.go`, replace the `Adapter` interface:

```go
// Adapter fetches the current job listings for a company.
// Required — implemented by all adapters.
type Adapter interface {
	FetchJobs(ctx context.Context, companyID string) (FetchJobsResponse, error)
	CareersURL(id string) string
}
```

- [ ] **Step 2: Verify the build fails with missing method errors**

```bash
cd backend && go build ./...
```

Expected: compile errors for greenhouse, lever, ashby, workday, workable, chromedp — each missing `CareersURL`. This confirms the interface is wired correctly before we implement.

---

### Task 2: Greenhouse — add `listURL`, update `FetchJobs`, implement `CareersURL`

**Files:**
- Modify: `backend/internal/ats_fetcher/greenhouse/greenhouse.go`
- Modify: `backend/internal/ats_fetcher/greenhouse/greenhouse_test.go`

- [ ] **Step 1: Write failing tests for `CareersURL` and slug-based `FetchJobs`**

Add to `backend/internal/ats_fetcher/greenhouse/greenhouse_test.go`:

```go
func TestGreenhouse_CareersURL(t *testing.T) {
	g := &Greenhouse{}
	got := g.CareersURL("stripe")
	want := "https://boards-api.greenhouse.io/v1/boards/stripe/jobs"
	if got != want {
		t.Errorf("CareersURL(%q) = %q, want %q", "stripe", got, want)
	}
}

func TestGreenhouse_FetchJobs_UsesSlug(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jobs":[]}`))
	}))
	defer srv.Close()

	g := &Greenhouse{listBaseURL: srv.URL}
	_, err := g.FetchJobs(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if capturedPath != "/v1/boards/stripe/jobs" {
		t.Errorf("API path = %q, want /v1/boards/stripe/jobs", capturedPath)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd backend && go test ./internal/ats_fetcher/greenhouse/... -run "TestGreenhouse_CareersURL|TestGreenhouse_FetchJobs_UsesSlug" -v
```

Expected: compile error — `listBaseURL` field does not exist yet.

- [ ] **Step 3: Update `greenhouse.go`**

Replace the full `Greenhouse` struct and add `listURL` + `CareersURL`. Update `FetchJobs` to use slug:

```go
type Greenhouse struct {
	// listBaseURL overrides the Greenhouse list API base — used in tests.
	// Leave empty for production (defaults to https://boards-api.greenhouse.io).
	listBaseURL string
	// contentBaseURL overrides the Greenhouse content API base — used in tests.
	// Leave empty for production (defaults to https://boards-api.greenhouse.io).
	contentBaseURL string
}

func (g *Greenhouse) listURL(slug string) string {
	base := "https://boards-api.greenhouse.io"
	if g.listBaseURL != "" {
		base = g.listBaseURL
	}
	return fmt.Sprintf("%s/v1/boards/%s/jobs", base, slug)
}

func (g *Greenhouse) contentURL(slug, id string) string {
	base := "https://boards-api.greenhouse.io"
	if g.contentBaseURL != "" {
		base = g.contentBaseURL
	}
	return fmt.Sprintf("%s/v1/boards/%s/jobs/%s", base, slug, id)
}

// CareersURL returns the Greenhouse job board API URL for a given company slug.
func (g *Greenhouse) CareersURL(id string) string {
	return fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs", id)
}
```

Update `FetchJobs` to take a slug and construct the URL:

```go
func (g *Greenhouse) FetchJobs(ctx context.Context, companyID string) (types.FetchJobsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.listURL(companyID), nil)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("greenhouse: build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("greenhouse: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return types.FetchJobsResponse{}, fmt.Errorf("greenhouse: unexpected status %d", resp.StatusCode)
	}

	var raw fetchJobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("greenhouse: decode: %w", err)
	}

	return raw.toFetchJobsResponse(), nil
}
```

- [ ] **Step 4: Update existing `FetchJobs` tests to use `listBaseURL` and slug**

In `greenhouse_test.go`, update all existing `FetchJobs` tests that pass `srv.URL` directly. Replace:
```go
g := &Greenhouse{}
result, err := g.FetchJobs(context.Background(), srv.URL)
```
With:
```go
g := &Greenhouse{listBaseURL: srv.URL}
result, err := g.FetchJobs(context.Background(), "stripe")
```

Apply this change to: `TestGreenhouse_FetchJobs`, `TestGreenhouse_FetchJobs_Empty`, `TestGreenhouse_FetchJobs_HTTPError`, `TestGreenhouse_FetchJobs_MalformedJSON`.

- [ ] **Step 5: Run all Greenhouse tests**

```bash
cd backend && go test ./internal/ats_fetcher/greenhouse/... -v
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/ats_fetcher/greenhouse/greenhouse.go backend/internal/ats_fetcher/greenhouse/greenhouse_test.go
git commit -m "feat(greenhouse): add CareersURL, switch FetchJobs to slug-based URL construction"
```

---

### Task 3: Lever — implement `CareersURL`

**Files:**
- Modify: `backend/internal/ats_fetcher/lever/lever.go`
- Modify: `backend/internal/ats_fetcher/lever/lever_test.go`

Lever's `FetchJobs` already takes a slug and constructs its own URL — no change needed there.

- [ ] **Step 1: Write failing test for `CareersURL`**

Add to `backend/internal/ats_fetcher/lever/lever_test.go`:

```go
func TestLever_CareersURL(t *testing.T) {
	l := &Lever{}
	got := l.CareersURL("netlify")
	want := "https://jobs.lever.co/netlify"
	if got != want {
		t.Errorf("CareersURL(%q) = %q, want %q", "netlify", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd backend && go test ./internal/ats_fetcher/lever/... -run TestLever_CareersURL -v
```

Expected: compile error — `CareersURL` not defined.

- [ ] **Step 3: Implement `CareersURL` in `lever.go`**

Add after the `apiURL` method:

```go
// CareersURL returns the public Lever jobs page URL for a given company slug.
func (l *Lever) CareersURL(id string) string {
	return "https://jobs.lever.co/" + id
}
```

- [ ] **Step 4: Run all Lever tests**

```bash
cd backend && go test ./internal/ats_fetcher/lever/... -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/ats_fetcher/lever/lever.go backend/internal/ats_fetcher/lever/lever_test.go
git commit -m "feat(lever): implement CareersURL"
```

---

### Task 4: Ashby — remove `SlugFromURL`, simplify `FetchJobs`, implement `CareersURL`

**Files:**
- Modify: `backend/internal/ats_fetcher/ashby/ashby.go`
- Modify: `backend/internal/ats_fetcher/ashby/ashby_test.go`

- [ ] **Step 1: Write failing test for `CareersURL`**

Add to `backend/internal/ats_fetcher/ashby/ashby_test.go`:

```go
func TestAshby_CareersURL(t *testing.T) {
	a := &Ashby{}
	got := a.CareersURL("openai")
	want := "https://jobs.ashbyhq.com/openai"
	if got != want {
		t.Errorf("CareersURL(%q) = %q, want %q", "openai", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd backend && go test ./internal/ats_fetcher/ashby/... -run TestAshby_CareersURL -v
```

Expected: compile error — `CareersURL` not defined.

- [ ] **Step 3: Remove `SlugFromURL`, simplify `FetchJobs`, add `CareersURL`**

In `ashby.go`:

**Delete** the entire `SlugFromURL` function (lines that define `func SlugFromURL(rawURL string) string`).

**Update `FetchJobs`** — replace `slug := SlugFromURL(companyID)` with `slug := companyID`:

```go
func (a *Ashby) FetchJobs(ctx context.Context, companyID string) (types.FetchJobsResponse, error) {
	slug := companyID  // companyID is now always a bare slug
	payload := map[string]any{
		// ... rest unchanged
```

**Add `CareersURL`** after the `restURL` method:

```go
// CareersURL returns the public Ashby hosted jobs page URL for a given company slug.
func (a *Ashby) CareersURL(id string) string {
	return "https://jobs.ashbyhq.com/" + id
}
```

- [ ] **Step 4: Remove stale `SlugFromURL` tests, remove full-URL slug extraction test**

In `ashby_test.go`, delete:
- `TestSlugFromURL` (function no longer exists)
- `TestAshby_FetchJobs_SlugExtractedFromFullURL` (behaviour no longer supported — crawler passes slug directly)

- [ ] **Step 5: Run all Ashby tests**

```bash
cd backend && go test ./internal/ats_fetcher/ashby/... -v
```

Expected: all pass. The removed tests no longer exist.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/ats_fetcher/ashby/ashby.go backend/internal/ats_fetcher/ashby/ashby_test.go
git commit -m "feat(ashby): implement CareersURL, remove SlugFromURL, use companyID directly as slug"
```

---

### Task 5: Workday, Workable, Chromedp — implement `CareersURL` stubs

**Files:**
- Modify: `backend/internal/ats_fetcher/workday/workday.go`
- Modify: `backend/internal/ats_fetcher/workable/workable.go`
- Modify: `backend/internal/ats_fetcher/chromedp/chromedp.go`

These adapters scrape arbitrary URLs — there is no standard slug-to-URL pattern. `CareersURL` returns `""` to satisfy the interface.

- [ ] **Step 1: Add `CareersURL` stub to each file**

`workday/workday.go` — add:
```go
func (w *Workday) CareersURL(id string) string { return "" }
```

`workable/workable.go` — add:
```go
func (w *Workable) CareersURL(id string) string { return "" }
```

`chromedp/chromedp.go` — add:
```go
func (c *Chromedp) CareersURL(id string) string { return "" }
```

- [ ] **Step 2: Verify the build passes**

```bash
cd backend && go build ./...
```

Expected: no errors. All adapters now satisfy the `Adapter` interface.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/ats_fetcher/workday/workday.go backend/internal/ats_fetcher/workable/workable.go backend/internal/ats_fetcher/chromedp/chromedp.go
git commit -m "feat(adapters): implement CareersURL stub for workday, workable, chromedp"
```

---

### Task 6: Crawler — pass `company.ID` instead of `company.URL`

**Files:**
- Modify: `backend/internal/worker/job_listing_crawler/engine.go`

- [ ] **Step 1: Update `scanCompany` to pass `company.ID`**

In `backend/internal/worker/job_listing_crawler/engine.go`, change line 78:

```go
// Before:
result, err := adp.FetchJobs(ctx, company.URL)

// After:
result, err := adp.FetchJobs(ctx, company.ID)
```

- [ ] **Step 2: Build and run all backend tests**

```bash
cd backend && go build ./... && go test ./...
```

Expected: all pass. No test references `company.URL` being passed to `FetchJobs`.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/worker/job_listing_crawler/engine.go
git commit -m "fix(crawler): pass company.ID (slug) to FetchJobs instead of company.URL"
```

---

### Task 7: Migration — normalize existing Lever and Ashby company URLs

**Files:**
- Create: `backend/migrations/013_normalize_company_urls.sql`

- [ ] **Step 1: Create the migration file**

Create `backend/migrations/013_normalize_company_urls.sql`:

```sql
-- +goose Up
-- Lever: was https://api.lever.co/v0/postings/{id}, now public page URL
UPDATE companies
SET url = 'https://jobs.lever.co/' || id
WHERE adapter_type = 'lever';

-- Ashby: was https://api.ashbyhq.com/posting-api/job-board/{id}, now public page URL
UPDATE companies
SET url = 'https://jobs.ashbyhq.com/' || id
WHERE adapter_type = 'ashby';

-- +goose Down
UPDATE companies
SET url = 'https://api.lever.co/v0/postings/' || id
WHERE adapter_type = 'lever';

UPDATE companies
SET url = 'https://api.ashbyhq.com/posting-api/job-board/' || id
WHERE adapter_type = 'ashby';
```

- [ ] **Step 2: Run the migration**

```bash
cd backend && go run cmd/migrate/main.go
```

Expected: output shows migration 013 applied.

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/013_normalize_company_urls.sql
git commit -m "migration: normalize Lever and Ashby company URLs to public page format"
```

---

### Task 8: Frontend — update URL templates

**Files:**
- Modify: `frontend/src/components/companies/AddCompanyModal.tsx`

- [ ] **Step 1: Update `URL_TEMPLATES` in `AddCompanyModal.tsx`**

In `frontend/src/components/companies/AddCompanyModal.tsx`, replace lines 19–24:

```ts
const URL_TEMPLATES: Partial<Record<string, (id: string) => string>> = {
  greenhouse: id => `https://boards-api.greenhouse.io/v1/boards/${id}/jobs`,
  lever:      id => `https://jobs.lever.co/${id}`,
  ashby:      id => `https://jobs.ashbyhq.com/${id}`,
  workable:   id => `https://${id}.workable.com/spi/v3/jobs`,
}
```

- [ ] **Step 2: Verify the frontend builds**

```bash
cd frontend && npm run build
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/companies/AddCompanyModal.tsx
git commit -m "fix(frontend): update Lever and Ashby URL templates to public careers page format"
```

---

## Verification

After all tasks are complete:

```bash
# Backend: full test suite
cd backend && go test ./...

# Trigger a scan for a Lever company and confirm it succeeds
# (check scan log in DB or company last_scan_status in UI)
```
