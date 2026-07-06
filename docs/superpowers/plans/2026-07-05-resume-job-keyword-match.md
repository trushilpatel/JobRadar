# Resume–Job Keyword Match (No-LLM) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the LLM entirely from resume keyword handling; broaden the enricher's job-keyword extraction; move resume↔job keyword matching to the frontend as a plain client-side computation; add a scoped frontend cache-TTL config.

**Architecture:** Backend now only _serves data_ — a resume's extracted plain text (`resumes.extracted_text`, populated synchronously at upload via the existing non-LLM PDF/DOCX parser) and a job's canonical keywords (`job_tech_tags`, broadened past pure tech-stack). The frontend computes matching (word-boundary regex, auto-pick, one-line summary) in a pure `lib/` utility called from the page that already composes the resumes and apply-queue features.

**Tech Stack:** Go 1.x, GORM, chi, Google Wire, goose migrations (backend) · React 19, TypeScript, TanStack Query, Vitest (frontend).

**Design doc:** `docs/superpowers/specs/2026-07-05-resume-job-keyword-match-design.md`

---

## Part A — Enricher & techtag broadening (independent of everything else)

### Task A1: Broaden `techtag.Clean`'s allow-list

**Files:**

- Modify: `backend/service/job/techtag/clean.go`
- Modify: `backend/service/job/techtag/clean_test.go`

- [ ] **Step 1: Update the failing-first test expectations**

Replace `TestClean_BlocklistDropsNoise` and `TestClean_MixedNoiseAndValid` in `clean_test.go` — the terms that are moving out of the blocklist must no longer be asserted as dropped:

```go
// TestClean_BlocklistDropsNoise — blocklist terms are dropped entirely
// regardless of input casing. Methodologies, tools, soft skills, and domain
// terms are no longer blocked (see TestClean_CanonicalMappingNormalizesVariants
// and TestClean_AllowsBroadenedCategories) — only genuine noise remains here.
func TestClean_BlocklistDropsNoise(t *testing.T) {
	noise := []string{
		"microsoft office", "excel", "word", "powerpoint",
		"windows", "macos", "unix",
		"api", "apis", "sdk", "json", "xml", "html", "css", "sql",
		"nosql", "version control",
	}
	got := techtag.Clean(noise)
	if len(got) != 0 {
		t.Errorf("expected all noise filtered, got %v", got)
	}
}

// TestClean_AllowsBroadenedCategories — methodologies, non-dev tools/platforms,
// soft skills, and certifications/domain terms now survive Clean, with
// canonical normalization applied where a mapping exists.
func TestClean_AllowsBroadenedCategories(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"agile", "Agile"},
		{"Scrum", "Scrum"},
		{"ci/cd", "CI/CD"},
		{"ci cd", "CI/CD"},
		{"microservices", "Microservices"},
		{"devops", "DevOps"},
		{"leadership", "Leadership"},
		{"communication", "Communication"},
		{"jira", "Jira"},
		{"figma", "Figma"},
		{"salesforce", "Salesforce"},
		{"aws certified", "AWS Certified"},
		{"pmp", "PMP"},
		{"hipaa", "HIPAA"},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			got := techtag.Clean([]string{c.raw})
			if len(got) != 1 || got[0] != c.want {
				t.Errorf("Clean(%q) = %v, want [%q]", c.raw, got, c.want)
			}
		})
	}
}

// TestClean_MixedNoiseAndValid — realistic broadened enricher output: some
// valid tech, some valid non-tech keywords, some duplicates, some genuine
// noise; output preserves only allowed canonical terms in input order.
func TestClean_MixedNoiseAndValid(t *testing.T) {
	in := []string{
		"Go", "agile", "REST APIs", "Kubernetes", "k8s",
		"communication", "PostgreSQL", "postgres", "microservices",
		"React", "reactjs", "SQL", "GraphQL", "jira",
	}
	got := techtag.Clean(in)
	want := []string{
		"Go", "Agile", "Kubernetes", "Communication", "PostgreSQL",
		"Microservices", "React", "GraphQL", "Jira",
	}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pos %d: want %q got %q", i, want[i], got[i])
		}
	}
}
```

Note: `"rest apis"` still passes through unmapped (kept as `"REST APIs"` verbatim, no canonical entry) — it is not blocklisted after this change since architecture patterns like REST are now an allowed "methodology" category. This matches the `want` slice above (`"REST APIs"` is absent from `want` only because it's not in `in` a second time — walk through `in` again: `"REST APIs"` @ index 2 IS in `in`, so it must appear in `want`). Fix: add `"REST APIs"` to `want` at the correct position (after `"Agile"`, before `"Kubernetes"`), i.e. `want := []string{"Go", "Agile", "REST APIs", "Kubernetes", "Communication", "PostgreSQL", "Microservices", "React", "GraphQL", "Jira"}`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./service/job/techtag/... -run TestClean -v`
Expected: FAIL — `TestClean_BlocklistDropsNoise` still lists agile/scrum/etc as noise (old blocklist), `TestClean_AllowsBroadenedCategories` fails because those terms are still blocked, `TestClean_MixedNoiseAndValid` fails on the new expected output.

- [ ] **Step 3: Update `blocklist` and `canonical` in `clean.go`**

Replace the `canonical` map's closing and the `blocklist` map with:

```go
	"spring":        "Spring",
	"spring boot":   "Spring Boot",
	"springboot":    "Spring Boot",
	// Methodologies / practices
	"agile":                  "Agile",
	"scrum":                  "Scrum",
	"kanban":                 "Kanban",
	"waterfall":              "Waterfall",
	"lean":                   "Lean",
	"rest":                   "REST",
	"restful":                "REST",
	"soap":                   "SOAP",
	"mvc":                    "MVC",
	"mvvm":                   "MVVM",
	"microservices":          "Microservices",
	"serverless":              "Serverless",
	"solid":                  "SOLID",
	"oop":                    "OOP",
	"object-oriented":        "OOP",
	"functional programming": "Functional Programming",
	"ci/cd":                  "CI/CD",
	"ci cd":                  "CI/CD",
	"devops":                 "DevOps",
	// Non-dev tools / platforms
	"jira":         "Jira",
	"confluence":   "Confluence",
	"figma":        "Figma",
	"salesforce":   "Salesforce",
	"tableau":      "Tableau",
	// Soft skills
	"communication":     "Communication",
	"leadership":        "Leadership",
	"teamwork":          "Teamwork",
	"collaboration":     "Collaboration",
	"stakeholder management": "Stakeholder Management",
	"mentorship":        "Mentorship",
	"problem solving":   "Problem Solving",
	"problem-solving":   "Problem Solving",
	"critical thinking": "Critical Thinking",
	// Certifications / domain terms
	"aws certified": "AWS Certified",
	"pmp":           "PMP",
	"hipaa":         "HIPAA",
	"fintech":       "Fintech",
	"healthcare":    "Healthcare",
}

// blocklist contains lowercase terms that carry no matching signal at all —
// generic office tools, generic OS names, and bare acronyms too broad to be
// meaningful on their own (e.g. "api" alone, vs. a specific tool/platform
// name). Methodologies, non-dev tools, soft skills, and certification/domain
// terms are deliberately NOT blocked here — see `canonical` above — since
// broadened job-keyword extraction now needs them for resume matching.
var blocklist = map[string]bool{
	// Generic office tools
	"microsoft office": true,
	"excel":            true,
	"word":              true,
	"powerpoint":       true,
	"google docs":      true,
	"google sheets":    true,
	// Generic OS (not specific enough)
	"windows": true,
	"macos":   true,
	"unix":    true,
	// Other noise
	"api":             true,
	"apis":            true,
	"sdk":             true,
	"json":            true,
	"xml":             true,
	"html":            true,
	"css":             true,
	"sql":             true,
	"nosql":           true,
	"version control": true,
}
```

Also update the package doc comment at the top of the file from "It maps variant spellings to canonical names and drops non-tech terms." to "It maps variant spellings to canonical names and drops generic noise — see `canonical` and `blocklist` for what counts as noise vs. an allowed keyword (which now spans tech stack, methodologies, tools, soft skills, and certifications)."

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./service/job/techtag/... -v`
Expected: PASS — all `TestClean_*` tests green.

- [ ] **Step 5: Commit**

```bash
git add backend/service/job/techtag/clean.go backend/service/job/techtag/clean_test.go
git commit -m "feat(techtag): broaden allow-list to methodologies, tools, soft skills, domain terms"
```

### Task A2: Broaden the enricher prompt's `tech_stack` field instructions

**Files:**

- Modify: `backend/service/llm/prompts/Enrich.go`
- Modify: `backend/service/llm/prompts/Enrich_test.go`

- [ ] **Step 1: Check the existing test for a hard assertion on the old exclusion wording**

Run: `grep -n "tech_stack\|Exclude" backend/service/llm/prompts/Enrich_test.go`. If a test asserts the prompt contains the string `"Exclude: methodologies"` or similar, update it to assert the new inclusion wording instead (see Step 3). If no such assertion exists, skip to Step 3 directly (no test change needed beyond confirming the prompt still builds).

- [ ] **Step 2: Run existing enrich prompt tests to confirm current baseline**

Run: `cd backend && go test ./service/llm/prompts/... -run TestBuildEnrichBatch -v`
Expected: PASS (baseline, before the prompt text changes).

- [ ] **Step 3: Update the `tech_stack` field instruction in `BuildEnrichBatch`**

In `Enrich.go`, replace the `tech_stack` line inside the returned prompt string:

```go
- tech_stack: array of keywords explicitly named in the posting, across these categories — (1) programming languages, frameworks, databases, cloud platforms, and developer tools (Go, Python, React, PostgreSQL, AWS, Docker, Terraform, Git); (2) methodologies and practices (Agile, Scrum, Kanban, CI/CD, TDD, Microservices, MVC, REST); (3) non-developer tools and platforms explicitly named (Jira, Confluence, Figma, Salesforce, Tableau); (4) soft skills explicitly named (Leadership, Communication, Stakeholder management, Mentorship); (5) certifications and domain terms explicitly named (AWS Certified, PMP, HIPAA, fintech, healthcare). Exclude only generic, non-specific terms: generic office tools (Excel, Word, Microsoft Office), bare acronyms with no specific referent (API, SDK, JSON, HTML on their own), and generic OS names (Windows, macOS). Use canonical names (e.g. "PostgreSQL" not "Postgres", "React" not "ReactJS", "CI/CD" not "continuous integration").
```

- [ ] **Step 4: If Step 1 found an assertion to update, update it now; otherwise add one**

If `Enrich_test.go` has no test asserting on the `tech_stack` instruction content, add:

```go
func TestBuildEnrichBatch_TechStackInstructionIncludesBroadenedCategories(t *testing.T) {
	prompt := prompts.BuildEnrichBatch([]string{"Software Engineer"}, []dto.JobPostEnrichInput{{ID: "j1", RawText: "text"}})
	for _, term := range []string{"methodologies", "Agile", "Jira", "Leadership", "certifications"} {
		if !strings.Contains(prompt, term) {
			t.Errorf("expected tech_stack instruction to mention %q, got prompt: %s", term, prompt)
		}
	}
	if strings.Contains(prompt, "Exclude: methodologies") {
		t.Error("expected the old blanket methodology exclusion wording to be gone")
	}
}
```

(Add `"strings"` to the test file's imports if not already present.)

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd backend && go test ./service/llm/prompts/... -run TestBuildEnrichBatch -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/service/llm/prompts/Enrich.go backend/service/llm/prompts/Enrich_test.go
git commit -m "feat(enricher): broaden tech_stack extraction to methodologies, tools, soft skills, domain terms"
```

---

## Part B — Migration

### Task B1: Migration 090 — drop `keywords`, add `extracted_text`

**Files:**

- Create: `backend/migrations/090_resume_extracted_text.sql`

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
-- Replaces LLM-derived resumes.keywords with a plain-text column populated by
-- the (non-LLM) PDF/DOCX text extractor at upload time. Matching against a
-- job's keywords now happens client-side against this raw text — see
-- docs/superpowers/specs/2026-07-05-resume-job-keyword-match-design.md.
-- No backfill for pre-existing resumes (explicit product decision) — they
-- get extracted_text = '' until re-uploaded.
ALTER TABLE resumes DROP COLUMN IF EXISTS keywords;
ALTER TABLE resumes ADD COLUMN extracted_text TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE resumes DROP COLUMN IF EXISTS extracted_text;
ALTER TABLE resumes ADD COLUMN keywords JSONB NOT NULL DEFAULT '[]'::jsonb;
```

- [ ] **Step 2: Apply the migration locally and verify**

Run: `cd backend && doppler run -- go run ./apps/migrate`
Expected: migration `090_resume_extracted_text` applies cleanly with no errors.

Run: `psql "$(grep '^SUPABASE_DB_URL=' /Users/trushil/Projects/JobRadar/.env | cut -d= -f2-)" -c "\d resumes"` (or the project's documented DB-access path if `psql` is unavailable locally) and confirm `extracted_text` exists and `keywords` is gone.

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/090_resume_extracted_text.sql
git commit -m "feat(resume): migrate resumes.keywords to extracted_text"
```

---

## Part C — textextract: normalize whitespace for matching

### Task C1: Collapse all whitespace (including newlines) in `ExtractText`'s output

**Files:**

- Modify: `backend/service/resume/textextract/extract.go`
- Modify: `backend/service/resume/textextract/extract_test.go`

- [ ] **Step 1: Write the failing test**

Add to `extract_test.go`:

```go
func TestExtractText_CollapsesLineBreaksForMultiWordMatching(t *testing.T) {
	// A <w:br/> forces a line break between "machine" and "learning" — without
	// full whitespace collapsing, the stray newline would prevent a
	// multi-word job keyword like "machine learning" from matching this
	// resume's extracted text on the frontend.
	inner := "Skilled in machine<w:br/>learning and distributed systems with five years of experience."
	data := buildTestDocx(t, docxParagraph(inner))

	text, err := textextract.ExtractText(filetype.ContentTypeDOCX, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if strings.Contains(text, "\n") {
		t.Fatalf("expected no embedded newlines in extracted text, got %q", text)
	}
	if !strings.Contains(text, "machine learning") {
		t.Fatalf("expected the line-broken phrase to join into one matchable string, got %q", text)
	}
}
```

Note: `docxParagraph` wraps `text` in a single `<w:t>` element, so the literal `<w:br/>` inside `inner` will be preserved as raw XML in the document body since it's outside the `<w:t>` tag boundaries in this test helper's simple template — confirm by checking `docxParagraph`'s definition (`` `...<w:t>` + text + `</w:t>...` ``); since `<w:br/>` here is embedded _inside_ the text argument, it lands _inside_ `<w:t>...</w:t>`, meaning it's literal text content, not a real `<w:br/>` element, and `docxBreakTag`'s regex (`` `<w:(br|tab|cr)\s*/>` ``) will still match it correctly since regex matching doesn't care whether it's "real" XML structure — the byte pattern `<w:br/>` appears verbatim in the XML either way. This is a valid test of the break-tag substitution path.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./service/resume/textextract/... -run TestExtractText_CollapsesLineBreaksForMultiWordMatching -v`
Expected: FAIL — current `collapseWhitespace` only folds `[ \t]+`, so if `docxBreakTag` inserted a literal space this specific case may already pass via the break-tag substitution; the meaningful failing case is a bare PDF-style `\n` between words that never goes through `docxBreakTag`. Replace the test's DOCX example with a **PDF** case instead, which more directly exercises `collapseWhitespace` on a real embedded newline:

```go
func TestExtractText_CollapsesNewlinesForMultiWordMatching(t *testing.T) {
	// extractPDF can yield real "\n" characters between wrapped lines (unlike
	// the DOCX path, which never emits raw text containing "\n" unless the
	// XML itself does) — collapseWhitespace must fold these too so a
	// multi-word job keyword can match across what was originally a
	// PDF line wrap.
	if !strings.Contains(strings.ReplaceAll("a\nb", "\n", " "), "a b") {
		t.Fatal("sanity check: ReplaceAll must fold newlines to spaces")
	}
}
```

Actually, simplest and most direct: unit test `collapseWhitespace`'s behavior isn't exported, so test it through `ExtractText` using the DOCX path with a literal `\n` character inside the paragraph text (not a `<w:br/>` tag) — DOCX XML can legitimately contain a raw `\n` byte inside a `<w:t>` element's text content, which is exactly the "stray newline mid-phrase" scenario the design doc calls out. Use this test instead of the two drafts above:

```go
func TestExtractText_CollapsesEmbeddedNewlinesForMultiWordMatching(t *testing.T) {
	inner := "Skilled in machine\nlearning and distributed systems with five years of experience."
	data := buildTestDocx(t, docxParagraph(inner))

	text, err := textextract.ExtractText(filetype.ContentTypeDOCX, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if strings.Contains(text, "\n") {
		t.Fatalf("expected no embedded newlines in extracted text, got %q", text)
	}
	if !strings.Contains(text, "machine learning") {
		t.Fatalf("expected the newline-broken phrase to join into one matchable string, got %q", text)
	}
}
```

Delete the two draft test functions above and keep only this one. Run: `cd backend && go test ./service/resume/textextract/... -run TestExtractText_CollapsesEmbeddedNewlinesForMultiWordMatching -v`
Expected: FAIL — current `collapseWhitespace` regex `[ \t]+` does not match `\n`, so the output still contains `"machine\nlearning"`, not `"machine learning"`.

- [ ] **Step 3: Update `collapseWhitespace` to fold all whitespace**

In `extract.go`, add a new regex and change `collapseWhitespace`'s implementation (leave `docxWhitespace` as-is — it's still used internally by `extractDOCX` for its own tag-stripping cleanup before the final pass):

```go
// allWhitespace collapses any run of whitespace — spaces, tabs, newlines —
// into a single space. The sole consumer of ExtractText's output is
// client-side keyword matching against a job's keywords, which needs one
// clean string rather than preserved paragraph structure; a stray line break
// from PDF line-wrapping or DOCX markup must never split a multi-word
// keyword like "machine learning" across an unmatched newline.
var allWhitespace = regexp.MustCompile(`\s+`)

// collapseWhitespace normalizes every run of whitespace (including newlines)
// to a single space.
func collapseWhitespace(s string) string {
	return allWhitespace.ReplaceAllString(s, " ")
}
```

Remove the old `collapseWhitespace` function body (the one delegating to `docxWhitespace`) and its now-stale doc comment ("leaving newlines intact so paragraph structure survives for the LLM prompt").

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd backend && go test ./service/resume/textextract/... -v`
Expected: PASS — all tests including the new one, and all pre-existing tests (none of them assert newlines are preserved, so none regress).

- [ ] **Step 5: Commit**

```bash
git add backend/service/resume/textextract/extract.go backend/service/resume/textextract/extract_test.go
git commit -m "fix(textextract): collapse embedded newlines so multi-word keywords can match"
```

---

## Part D — Resume domain: remove the LLM

### Task D1: Add `ExtractedText` to `UploadInput`

**Files:**

- Modify: `backend/service/resume/dto/UploadInput.go`

- [ ] **Step 1: Add the field**

```go
// UploadInput carries an upload request through the decorator chain.
// ContentType, Extension, ID, StorageKey, and ExtractedText start empty and
// are filled in by ValidationService (content-type sniffing) and ImplService
// (ID, storage key generation, and synchronous text extraction) before
// reaching the repository.
type UploadInput struct {
	ID               string
	Label            string
	OriginalFilename string
	ContentType      string
	Extension        string
	RawBytes         []byte
	DeclaredSize     int64
	StorageKey       string
	ExtractedText    string
}
```

- [ ] **Step 2: Build to confirm no compile error**

Run: `cd backend && go build ./service/resume/...`
Expected: PASS (struct field addition alone can't break anything yet).

- [ ] **Step 3: Commit**

```bash
git add backend/service/resume/dto/UploadInput.go
git commit -m "feat(resume): add ExtractedText field to UploadInput"
```

### Task D2: Replace `Resume.Keywords` with `Resume.ExtractedText`

**Files:**

- Modify: `backend/service/resume/models/resume.go`

- [ ] **Step 1: Update the model**

```go
// Package resumemodel holds the GORM model for the resumes table.
package resumemodel

import "time"

// Resume mirrors the `resumes` table (migration 086, extracted_text added in
// 090, replacing the earlier LLM-derived `keywords` column). Each row is a
// single uploaded file — no versioning in this phase.
// StorageKey is deliberately excluded from JSON (json:"-"): it's an internal
// object-storage path, never something a client should see or depend on.
//
// ExtractedText holds the plain text produced by service/resume/textextract
// at upload time (PDF/DOCX -> text, no LLM). It is never displayed to the
// user directly — its sole consumer is client-side keyword matching against
// a job's keywords (see docs/superpowers/specs/2026-07-05-resume-job-keyword-match-design.md).
type Resume struct {
	ID               string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	UserID           string    `gorm:"column:user_id;not null;index"                  json:"user_id"`
	Label            string    `gorm:"not null"                                       json:"label"`
	OriginalFilename string    `gorm:"column:original_filename;not null"              json:"original_filename"`
	StorageKey       string    `gorm:"column:storage_key;not null;uniqueIndex"        json:"-"`
	ContentType      string    `gorm:"column:content_type;not null"                   json:"content_type"`
	SizeBytes        int64     `gorm:"column:size_bytes;not null"                     json:"size_bytes"`
	CreatedAt        time.Time `gorm:"column:created_at;not null;default:now()"       json:"created_at"`
	ExtractedText    string    `gorm:"column:extracted_text;not null;default:''"      json:"extracted_text"`
}

func (Resume) TableName() string { return "resumes" }
```

Note `datatypes` import is dropped (no longer needed — `ExtractedText` is a plain `string`, not `datatypes.JSON`).

- [ ] **Step 2: Build (expect cascading errors — that's expected, fixed in the next tasks)**

Run: `cd backend && go build ./service/resume/... 2>&1 | head -40`
Expected: FAIL — `ResumeRepositoryService.go` still references `resumemodel.Resume{Keywords: ...}`-shaped code and `KeywordStore`/`UpdateKeywords`. This is expected; Task D3 fixes it.

- [ ] **Step 3: Commit** (allowed to commit a transiently-broken intermediate state within this feature branch, since the whole Part D is one atomic unit of work landing together — or squash Tasks D1–D6 into fewer commits if preferred; if committing per-task, note in the message that the package doesn't build yet)

```bash
git add backend/service/resume/models/resume.go
git commit -m "feat(resume): replace Keywords column with ExtractedText in model"
```

### Task D3: Simplify `ResumeRepositoryService.go` — persist `ExtractedText` on insert, drop `KeywordStore`

**Files:**

- Modify: `backend/service/resume/services/ResumeRepositoryService.go`

- [ ] **Step 1: Update `Upload` to persist `ExtractedText` and remove `KeywordStore`/`UpdateKeywords`/`marshalKeywords`**

Replace the `row := resumemodel.Resume{...}` construction inside `Upload`:

```go
	row := resumemodel.Resume{
		ID:               input.ID,
		UserID:           userID,
		Label:            input.Label,
		OriginalFilename: input.OriginalFilename,
		StorageKey:       input.StorageKey,
		ContentType:      input.ContentType,
		SizeBytes:        input.DeclaredSize,
		CreatedAt:        time.Now().UTC(),
		ExtractedText:    input.ExtractedText,
	}
```

Delete entirely: `ExtractKeywords` repo stub method (lines ~175-182 — "unreachable in the real chain" — no longer part of the interface, see Task D5), `NewKeywordStore` constructor, `UpdateKeywords` method, `marshalKeywords` helper, and the `var _ KeywordStore = (*repositoryService)(nil)` assertion at the bottom. Remove the now-unused `"encoding/json"` and `"gorm.io/datatypes"` imports if nothing else in the file uses them (grep first: `grep -n "json\.\|datatypes\." backend/service/resume/services/ResumeRepositoryService.go` — if no remaining hits outside the deleted code, remove both imports).

- [ ] **Step 2: Build**

Run: `cd backend && go build ./service/resume/services/... 2>&1 | head -40`
Expected: FAIL — `ResumeService.go` (Task D5) and `ResumeImplService.go` (Task D6) still reference the removed pieces. Continue to the next tasks before expecting a clean build.

- [ ] **Step 3: Commit**

```bash
git add backend/service/resume/services/ResumeRepositoryService.go
git commit -m "feat(resume): persist ExtractedText on upload, drop KeywordStore"
```

### Task D4: Update `ResumeRepositoryService_test.go`

**Files:**

- Modify: `backend/service/resume/tests/ResumeRepositoryService_test.go`

- [ ] **Step 1: Delete keyword-store tests and the unreachable-stub test**

Delete these test functions entirely: `TestResumeKeywordStore_UpdateKeywordsPersistsAndIsReadableViaGet`, `TestResumeKeywordStore_UpdateKeywordsWithEmptySliceStoresJSONArray`, `TestResumeKeywordStore_UpdateKeywordsMissingResumeReturnsNotFound`, `TestResumeKeywordStore_UpdateKeywordsReturnsInternalErrorOnDBFailure`, `TestResumeRepository_ExtractKeywordsIsUnreachableStub`.

- [ ] **Step 2: Add a test confirming `ExtractedText` round-trips through Upload/Get**

Add:

```go
func TestResumeRepository_UploadPersistsExtractedText(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)

	created, err := repo.Upload(context.Background(), "user-1", dto.UploadInput{
		ID: "r1", Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		StorageKey: "resumes/user-1/r1.pdf", DeclaredSize: 8,
		ExtractedText: "Backend engineer with Go and PostgreSQL experience.",
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if created.ExtractedText != "Backend engineer with Go and PostgreSQL experience." {
		t.Fatalf("expected ExtractedText to be persisted, got %q", created.ExtractedText)
	}

	got, err := repo.Get(context.Background(), "user-1", "r1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ExtractedText != created.ExtractedText {
		t.Fatalf("Get returned different ExtractedText: %q vs %q", got.ExtractedText, created.ExtractedText)
	}
}
```

- [ ] **Step 3: Update `newResumeTestDB`'s hand-written schema (if it exists in this test package) to use `extracted_text` instead of `keywords`**

Run: `grep -n "keywords\|CREATE TABLE resumes" backend/service/resume/tests/*.go` to find the schema definition. Update the hand-written `CREATE TABLE resumes (...)` (or `AutoMigrate(&resumemodel.Resume{})` — if it uses AutoMigrate on the model directly, no schema string to edit, the model change from Task D2 already covers it).

- [ ] **Step 4: Run the test file**

Run: `cd backend && go test ./service/resume/... -run TestResumeRepository -v 2>&1 | tail -60`
Expected: still failing at this point because `ResumeService.go`/`ResumeImplService.go` haven't been updated (Tasks D5-D6) — package won't compile yet. This is expected; proceed.

- [ ] **Step 5: Commit**

```bash
git add backend/service/resume/tests/ResumeRepositoryService_test.go
git commit -m "test(resume): cover ExtractedText persistence, drop KeywordStore tests"
```

### Task D5: Remove `ExtractKeywords`/`KeywordExtractor`/`KeywordStore` from the `Service` contract

**Files:**

- Modify: `backend/service/resume/services/ResumeService.go`

- [ ] **Step 1: Rewrite the file**

```go
// Package services holds the decorator chain for the resume domain.
package services

import (
	"context"
	"io"

	"github.com/trushilpatel/jobradar/service/resume/dto"
	resumemodel "github.com/trushilpatel/jobradar/service/resume/models"
)

type Service interface {
	List(ctx context.Context, userID string) ([]resumemodel.Resume, error)
	Get(ctx context.Context, userID, id string) (*resumemodel.Resume, error)
	Upload(ctx context.Context, userID string, input dto.UploadInput) (*resumemodel.Resume, error)
	Rename(ctx context.Context, userID, id, label string) (*resumemodel.Resume, error)
	Delete(ctx context.Context, userID, id string) error
	// Download returns a reader over the resume's object-storage bytes plus
	// its metadata row (for content-type/filename/size) — the REST layer
	// streams the reader directly to the client. Callers must Close() it.
	Download(ctx context.Context, userID, id string) (io.ReadCloser, *resumemodel.Resume, error)
}

type BaseService struct {
	next Service
}

func NewBaseService(next Service) BaseService {
	if next == nil {
		panic("resume.BaseService: next must not be nil — check wire setup")
	}
	return BaseService{next: next}
}

func (b *BaseService) List(ctx context.Context, userID string) ([]resumemodel.Resume, error) {
	return b.next.List(ctx, userID)
}

func (b *BaseService) Get(ctx context.Context, userID, id string) (*resumemodel.Resume, error) {
	return b.next.Get(ctx, userID, id)
}

func (b *BaseService) Upload(ctx context.Context, userID string, input dto.UploadInput) (*resumemodel.Resume, error) {
	return b.next.Upload(ctx, userID, input)
}

func (b *BaseService) Rename(ctx context.Context, userID, id, label string) (*resumemodel.Resume, error) {
	return b.next.Rename(ctx, userID, id, label)
}

func (b *BaseService) Delete(ctx context.Context, userID, id string) error {
	return b.next.Delete(ctx, userID, id)
}

func (b *BaseService) Download(ctx context.Context, userID, id string) (io.ReadCloser, *resumemodel.Resume, error) {
	return b.next.Download(ctx, userID, id)
}
```

This drops `KeywordExtractor`, `KeywordStore`, and `ExtractKeywords` (interface method + `BaseService` pass-through) entirely, and the now-unused `llmdto` import.

- [ ] **Step 2: Build**

Run: `cd backend && go build ./service/resume/... 2>&1 | head -60`
Expected: FAIL — `ResumeImplService.go`, `ResumeRestService.go`, `ResumeRestRouter.go` still implement/call the removed pieces. Continue to Task D6.

- [ ] **Step 3: Commit**

```bash
git add backend/service/resume/services/ResumeService.go
git commit -m "feat(resume): drop ExtractKeywords/KeywordExtractor/KeywordStore from the Service contract"
```

### Task D6: Rewrite `ResumeImplService.go` — synchronous extraction at upload, no LLM

**Files:**

- Modify: `backend/service/resume/services/ResumeImplService.go`

- [ ] **Step 1: Rewrite the file**

```go
package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/platform/objectstorage"
	"github.com/trushilpatel/jobradar/service/resume/dto"
	resumemodel "github.com/trushilpatel/jobradar/service/resume/models"
	"github.com/trushilpatel/jobradar/service/resume/textextract"
)

type implService struct {
	BaseService
	storage objectstorage.Storage
	log     logger.Logger
}

func NewImplService(next Service, storage objectstorage.Storage, log logger.Logger) Service {
	return &implService{
		BaseService: NewBaseService(next),
		storage:     storage,
		log:         log,
	}
}

// Upload extracts plain text from the file synchronously (no LLM — the
// PDF/DOCX parser in service/resume/textextract is a local, fast operation),
// then writes the file to object storage before creating the DB row — a
// crash between the two steps leaves an unreferenced (harmless, cheap)
// object in storage rather than a DB row pointing at nothing. If the DB
// insert itself fails (e.g. the per-user resume cap), the just-written
// object is deleted immediately since the failure is known synchronously.
//
// Extraction runs before the storage write so an unreadable file (scanned
// image PDF with no text layer, corrupt file) fails the upload itself with a
// clear error, rather than uploading successfully and having no matchable
// content later — unlike the old best-effort background LLM extraction this
// replaces, there is no silent "extraction never ran" state.
func (s *implService) Upload(ctx context.Context, userID string, input dto.UploadInput) (_ *resumemodel.Resume, err error) {
	ctx, span := tracing.Start(ctx, "resume.impl.Upload")
	defer tracing.End(span, &err)

	text, extractErr := textextract.ExtractText(input.ContentType, bytes.NewReader(input.RawBytes))
	if extractErr != nil {
		if errors.Is(extractErr, textextract.ErrShortContent) || errors.Is(extractErr, textextract.ErrUnsupportedContentType) {
			return nil, domainerrs.ErrBadRequest.
				WithMessage("resume impl: extract text: " + extractErr.Error()).
				WithExternal("could not read enough text from this file — try a different file")
		}
		return nil, domainerrs.ErrBadRequest.
			WithMessage("resume impl: extract text: " + extractErr.Error()).
			WithExternal("could not read this file — it may be corrupted or password-protected")
	}
	input.ExtractedText = text

	input.ID = uuid.NewString()
	input.StorageKey = fmt.Sprintf("resumes/%s/%s.%s", userID, input.ID, input.Extension)

	if putErr := s.storage.Put(ctx, input.StorageKey, bytes.NewReader(input.RawBytes), input.DeclaredSize, input.ContentType); putErr != nil {
		return nil, domainerrs.ErrInternal.WithMessage("resume impl: storage put: " + putErr.Error())
	}

	resume, err := s.next.Upload(ctx, userID, input)
	if err != nil {
		if delErr := s.storage.Delete(ctx, input.StorageKey); delErr != nil {
			s.log.Warn("resume impl: cleanup after failed create left an orphaned object",
				logger.Str(logger.FieldStorageKey, input.StorageKey), logger.Err(delErr))
		}
		return nil, err
	}
	return resume, nil
}

// Delete removes the DB row first, then best-effort deletes the object from
// storage. A storage-delete failure is logged but never blocks the
// user-visible delete — an orphaned object here is accepted Phase-0 tech
// debt (no reaper yet); a DB row kept alive by a failed storage delete would
// be the worse, user-visible failure mode.
func (s *implService) Delete(ctx context.Context, userID, id string) (err error) {
	ctx, span := tracing.Start(ctx, "resume.impl.Delete")
	defer tracing.End(span, &err)

	resume, err := s.next.Get(ctx, userID, id)
	if err != nil {
		return err
	}
	if delErr := s.next.Delete(ctx, userID, id); delErr != nil {
		return delErr
	}
	if storageErr := s.storage.Delete(ctx, resume.StorageKey); storageErr != nil {
		s.log.Warn("resume impl: storage object delete failed after DB delete succeeded — orphaned object",
			logger.Str(logger.FieldStorageKey, resume.StorageKey), logger.Err(storageErr))
	}
	return nil
}

func (s *implService) Download(ctx context.Context, userID, id string) (_ io.ReadCloser, _ *resumemodel.Resume, err error) {
	ctx, span := tracing.Start(ctx, "resume.impl.Download")
	defer tracing.End(span, &err)

	resume, err := s.next.Get(ctx, userID, id)
	if err != nil {
		return nil, nil, err
	}
	reader, _, getErr := s.storage.Get(ctx, resume.StorageKey)
	if getErr != nil {
		if errors.Is(getErr, objectstorage.ErrNotFound) {
			return nil, nil, domainerrs.ErrNotFound.WithMessage("resume impl: object missing from storage: " + getErr.Error())
		}
		return nil, nil, domainerrs.ErrInternal.WithMessage("resume impl: storage get: " + getErr.Error())
	}
	return reader, resume, nil
}

var _ Service = (*implService)(nil)
```

This drops `scheduleKeywordExtraction`, `runKeywordExtraction`, `ExtractKeywords`, the `extractor`/`keywordStore` fields and constructor params, and the `llmdto`/`resumecommon`/`techtag`/`runtime/debug` imports.

- [ ] **Step 2: Build**

Run: `cd backend && go build ./service/resume/... 2>&1 | head -60`
Expected: FAIL only in `ResumeRestService.go`/`ResumeRestRouter.go` (Task D7) and callers in `apps/api` (Part F). Resume package's own `services` sub-package should now build except for those two files.

- [ ] **Step 3: Commit**

```bash
git add backend/service/resume/services/ResumeImplService.go
git commit -m "feat(resume): synchronous non-LLM text extraction at upload, drop background LLM path"
```

### Task D7: Remove the `/resumes/{id}/keywords` endpoint

**Files:**

- Modify: `backend/service/resume/services/ResumeRestService.go`
- Modify: `backend/service/resume/services/ResumeRestRouter.go`

- [ ] **Step 1: Delete the `extractKeywords` handler from `ResumeRestService.go`**

Remove the entire `extractKeywords` method (the last method in the file).

- [ ] **Step 2: Delete the route registration from `ResumeRestRouter.go`**

```go
func (rr *RestRouter) Register(r chi.Router) {
	r.Get("/resumes", rr.handler.list)
	r.Post("/resumes", rr.handler.upload)
	r.Get("/resumes/{id}/download", rr.handler.download)
	r.Patch("/resumes/{id}", rr.handler.rename)
	r.Delete("/resumes/{id}", rr.handler.remove)
}
```

- [ ] **Step 3: Build**

Run: `cd backend && go build ./service/resume/... 2>&1 | head -60`
Expected: PASS (resume package's own services now build cleanly — `apps/api` wiring is still broken until Part F).

- [ ] **Step 4: Commit**

```bash
git add backend/service/resume/services/ResumeRestService.go backend/service/resume/services/ResumeRestRouter.go
git commit -m "feat(resume): remove POST /resumes/{id}/keywords endpoint"
```

### Task D8: Remove `KeywordExtractionTimeout` from resume constants

**Files:**

- Modify: `backend/service/resume/common/ResumeServiceConstants.go`

- [ ] **Step 1: Remove the constant and its `time` import**

```go
package common

const (
	// LabelMaxLen bounds the user-facing display label (upload and rename).
	LabelMaxLen = 200
	// OriginalFilenameMaxLen bounds the stored original filename.
	OriginalFilenameMaxLen = 255
	// ResumeLimitPerUser is the max number of resumes a single user may store.
	ResumeLimitPerUser = 50
	// MaxFileSizeBytes is the max accepted upload size (2MB), matching the
	// `resumes.size_bytes` CHECK constraint in migration 086.
	MaxFileSizeBytes = 2 * 1024 * 1024
)
```

- [ ] **Step 2: Build**

Run: `cd backend && go build ./service/resume/... 2>&1 | head -20`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/service/resume/common/ResumeServiceConstants.go
git commit -m "chore(resume): remove unused KeywordExtractionTimeout constant"
```

### Task D9: Rewrite `ResumeImplService_test.go`

**Files:**

- Modify: `backend/service/resume/tests/ResumeImplService_test.go`

- [ ] **Step 1: Delete the keyword-extraction fakes and dead helpers**

Delete: `fakeKeywordExtractor` struct + `newNoopKeywordExtractor` + its `ExtractResumeKeywords`/`calls` methods, `fakeKeywordStore` struct + `newFakeKeywordStore` + its methods, `chanLogger` struct + all its methods, `waitOrTimeout`. Keep `buildValidDocx` and the `validResumeText` constant — repurpose their doc comments (see Step 2).

- [ ] **Step 2: Update `buildValidDocx`'s doc comment**

```go
// buildValidDocx assembles a minimal but genuinely parseable docx (matching
// service/resume/textextract's own test fixture shape) so Upload's
// synchronous textextract.ExtractText step actually succeeds — Upload now
// requires real extractable text, so every Upload call in this file needs
// this instead of the placeholder PDF-header-only bytes used before this
// feature (which were never actually parsed, since extraction used to be
// backgrounded and best-effort).
func buildValidDocx(t *testing.T, text string) []byte {
```

- [ ] **Step 3: Delete the tests that no longer apply**

Delete entirely: `TestBaseService_ExtractKeywordsDelegatesToNext`, `TestResumeImpl_UploadSchedulesBackgroundKeywordExtractionAndPersistsResult`, `TestResumeImpl_UploadDoesNotBlockOnSlowExtraction`, `TestResumeImpl_UploadRecoversFromPanicInExtraction`, `TestResumeImpl_ExtractKeywords_Success`, `TestResumeImpl_ExtractKeywords_WrongUserReturnsNotFound`, `TestResumeImpl_ExtractKeywords_TextExtractionFailurePropagatesAsInternal`, `TestResumeImpl_ExtractKeywords_LLMErrorPropagatesAsInternal`, `TestResumeImpl_ExtractKeywords_StoreErrorPropagates`, `TestResumeImpl_ExtractKeywords_MissingResumeReturnsNotFound`, `TestResumeImpl_ExtractKeywords_StorageGetErrorPropagatesAsInternal`.

- [ ] **Step 4: Replace `TestResumeImpl_UploadExtractionFailureLeavesKeywordsEmpty` with a test of the new fail-fast behavior**

```go
func TestResumeImpl_UploadFailsWhenTextExtractionFails(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())

	// "%PDF-1.4" alone is too short/invalid for textextract to parse — Upload
	// must now fail synchronously rather than succeeding with empty content.
	_, err := impl.Upload(context.Background(), "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})
	if !errors.Is(err, domainerrs.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest when text extraction fails, got %v", err)
	}
	if storage.Has("resumes/user-1/") {
		t.Fatal("expected no object to be written to storage when extraction fails before the storage write")
	}
}

func TestResumeImpl_UploadPersistsExtractedText(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())

	resume, err := impl.Upload(context.Background(), "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.docx", ContentType: filetype.ContentTypeDOCX,
		Extension: "docx", RawBytes: buildValidDocx(t, validResumeText), DeclaredSize: 8,
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if resume.ExtractedText == "" {
		t.Fatal("expected Upload to persist the extracted text")
	}
	if !strings.Contains(resume.ExtractedText, "Backend engineer") {
		t.Fatalf("expected extracted text to contain the fixture content, got %q", resume.ExtractedText)
	}
}
```

(Add `"strings"` to imports if not already present — check first, since it likely already is via other test bodies.)

- [ ] **Step 5: Update every remaining `NewImplService(...)` call to drop the two removed args, and switch every `RawBytes: []byte("%PDF-1.4")` fixture to `buildValidDocx`**

For each of the following test functions, change the `NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())` call to `NewImplService(repo, storage, logger.NewNoop())`, and change every `dto.UploadInput{..., ContentType: "application/pdf", Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8}` to `dto.UploadInput{..., ContentType: filetype.ContentTypeDOCX, Extension: "docx", RawBytes: buildValidDocx(t, validResumeText), DeclaredSize: 8}` (adjusting `OriginalFilename` from `"f.pdf"`/`fmt.Sprintf("f.pdf", ...)`-style to `"f.docx"` accordingly):

- `TestResumeImpl_UploadWritesStorageBeforeDBRow`
- `TestResumeImpl_UploadCleansUpStorageWhenDBInsertFails` (both the seeding loop's 50 uploads and the final failing upload)
- `TestResumeImpl_DeleteRemovesDBRowAndStorageObject`
- `TestResumeImpl_DeleteForWrongUserReturnsNotFoundAndLeavesObjectIntact`
- `TestResumeImpl_DownloadReturnsReaderForOwner`
- `TestResumeImpl_DownloadForWrongUserReturnsNotFound`
- `TestResumeImpl_ListPassesThroughToRepo`
- `TestResumeImpl_RenamePassesThroughToRepo`
- `TestResumeImpl_UploadPropagatesStoragePutError`
- `TestResumeImpl_DeleteSucceedsEvenWhenStorageDeleteFails`
- `TestResumeImpl_DownloadPropagatesStorageError`
- `TestResumeImpl_DownloadMapsStorageNotFoundToDomainNotFound`
- `TestResumeImpl_DownloadForDeletedResumeReturnsNotFound`
- `TestNewImplService_PanicsOnNilNext` (no `RawBytes` to change here — just drop the two constructor args: `resumesvc.NewImplService(nil, objectstorage.NewMemoryStorage(), logger.NewNoop())`)

- [ ] **Step 6: Remove the now-unused `llmdto` import**

Since no test in this file references `llmdto.ResumeKeywordInput`/`Result` anymore, remove `llmdto "github.com/trushilpatel/jobradar/service/llm/dto"` from the import block. Also remove `"sync"` / `"sync/atomic"` / `"time"` if nothing else in the file uses them after the deletions (check with `grep -n "sync\.\|atomic\.\|time\." backend/service/resume/tests/ResumeImplService_test.go`).

- [ ] **Step 7: Run the full test file**

Run: `cd backend && go test ./service/resume/tests/... -run TestResumeImpl -v -race`
Expected: PASS — all remaining/updated tests green, race-clean.

- [ ] **Step 8: Commit**

```bash
git add backend/service/resume/tests/ResumeImplService_test.go
git commit -m "test(resume): rewrite ImplService tests for synchronous non-LLM extraction"
```

### Task D10: Confirm `ResumeValidationService_test.go` and `ResumeRestService_test.go` need no structural changes, delete the three `ExtractKeywords` REST tests

**Files:**

- Modify: `backend/service/resume/tests/ResumeRestService_test.go`

- [ ] **Step 1: Delete the three ExtractKeywords REST-layer tests**

Delete `TestResumeRest_ExtractKeywordsReturns200OnSuccess`, `TestResumeRest_ExtractKeywordsReturns404ForMissingResume`, `TestResumeRest_ExtractKeywordsReturns500OnExtractionFailure`. (These use a hand-rolled `fakeService` mock, not the real `ImplService`, so no other test in this file needs a fixture change — confirmed in research: `TestResumeRest_UploadReturns201OnSuccess` and its siblings mock `Service` directly and never invoke real text extraction.)

- [ ] **Step 2: Check `fakeService`'s struct definition for an `extractResult`/`extractErr` field used only by the deleted tests**

Run: `grep -n "extractResult\|extractErr\|ExtractKeywords" backend/service/resume/tests/*.go`. If `fakeService.ExtractKeywords(...)` method and `extractResult`/`extractErr` fields exist solely to satisfy the old `Service` interface, remove them now that `Service` no longer declares `ExtractKeywords` (Task D5 already dropped it from the interface, so `fakeService` implementing it is harmless-but-dead; remove for cleanliness since `Service` is now smaller and the compiler will flag `fakeService` as still satisfying it either way — but leaving dead fields is against the project's no-dead-code norms).

- [ ] **Step 3: Run the full resume test package**

Run: `cd backend && go test ./service/resume/... -v -race 2>&1 | tail -80`
Expected: PASS — every test in `service/resume/...` green, race-clean.

- [ ] **Step 4: Commit**

```bash
git add backend/service/resume/tests/ResumeRestService_test.go
git commit -m "test(resume): remove ExtractKeywords REST-layer tests"
```

---

## Part E — LLM domain cleanup (now-dead code after Part D)

### Task E1: Remove `ResumeKeywordExtractor` from the `Scorer` contract

**Files:**

- Modify: `backend/service/llm/services/LLMService.go`

- [ ] **Step 1: Rewrite the file**

```go
package services

import (
	"context"

	"github.com/trushilpatel/jobradar/service/llm/dto"
)

// JobsRelevanceFilter scores a batch of jobs and returns the IDs of relevant ones.
// Used by the filter worker.
type JobsRelevanceFilter interface {
	ScoreBatch(ctx context.Context, jobs []dto.JobsRelevanceFilterInput) (relevantIDs []string, err error)
}

// JobPostEnricher extracts structured data from one or more job postings in a
// single LLM call. Inputs and results are correlated by JobID — the LLM is
// instructed to return the same job_id on each result, and the caller maps
// results back by ID. A batch of size 1 is the natural single-job case.
//
// Used by the analyzer worker.
type JobPostEnricher interface {
	EnrichBatch(ctx context.Context, inputs []dto.JobPostEnrichInput) ([]*dto.JobPostEnrichResult, error)
}

// Scorer combines both interfaces. Returned by the factory — callers
// receive the narrower interface they actually need.
type Scorer interface {
	JobsRelevanceFilter
	JobPostEnricher
}
```

- [ ] **Step 2: Build (expect failures — fixed in the following steps of this task)**

Run: `cd backend && go build ./service/llm/... 2>&1 | head -60`
Expected: FAIL — `LLMGeminiService.go`, `LLMRateLimitService.go`, `LLMRouterService.go` still implement `ExtractResumeKeywords`, which is now an extra (harmless) method — Go doesn't error on extra methods, so the actual failures will instead come from the dto/prompt removal in later steps. Proceed to Step 3.

- [ ] **Step 3: Remove `ExtractResumeKeywords` from `LLMGeminiService.go`**

Delete the `// ── ExtractResumeKeywords ──` section: `resumeKeywordSchema` var and the `ExtractResumeKeywords` method.

- [ ] **Step 4: Remove `ExtractResumeKeywords` from `LLMRateLimitService.go`**

Delete the `ExtractResumeKeywords` method and `estimateResumeKeywordTokens` function. Delete the `resumeKeywordBaseTokens`/`resumeKeywordTokensPerChar` constants.

- [ ] **Step 5: Remove `ExtractResumeKeywords` from `LLMRouterService.go`**

Delete the `ExtractResumeKeywords` method (the whole function body shown in research, lines ~122-150ish through its closing brace).

- [ ] **Step 6: Delete `prompts/ResumeKeywords.go` and its test**

```bash
rm backend/service/llm/prompts/ResumeKeywords.go backend/service/llm/prompts/ResumeKeywords_test.go
```

- [ ] **Step 7: Delete the dto files**

```bash
rm backend/service/llm/dto/ResumeKeywordInput.go backend/service/llm/dto/ResumeKeywordResult.go
```

- [ ] **Step 8: Remove `ResumeKeywordMaxTextChars` from `common/constants.go`**

```go
package common

const (
	ProviderGemini = "gemini"
)
```

- [ ] **Step 9: Build**

Run: `cd backend && go build ./service/llm/... 2>&1 | head -60`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add backend/service/llm/services/LLMService.go backend/service/llm/services/LLMGeminiService.go \
        backend/service/llm/services/LLMRateLimitService.go backend/service/llm/services/LLMRouterService.go \
        backend/service/llm/common/constants.go
git rm backend/service/llm/prompts/ResumeKeywords.go backend/service/llm/prompts/ResumeKeywords_test.go \
       backend/service/llm/dto/ResumeKeywordInput.go backend/service/llm/dto/ResumeKeywordResult.go
git commit -m "feat(llm): remove ResumeKeywordExtractor — resume domain no longer depends on the LLM"
```

### Task E2: Remove dead LLM tests

**Files:**

- Modify: `backend/service/llm/services/LLMGeminiService_http_internal_test.go`
- Modify: `backend/service/llm/services/tests/LLMRateLimitService_test.go`
- Modify: `backend/service/llm/services/tests/LLMRouterService_test.go`

- [ ] **Step 1: Delete the 5 Gemini tests**

In `LLMGeminiService_http_internal_test.go`, delete: `TestGemini_ExtractResumeKeywords_EmptyTextReturnsValidationError`, `TestGemini_ExtractResumeKeywords_HappyPath`, `TestGemini_ExtractResumeKeywords_TruncatesOversizedText`, `TestGemini_ExtractResumeKeywords_RejectsUnparseableJSON`, `TestGemini_ExtractResumeKeywords_Maps429ToSentinel`, and the `// ─── ExtractResumeKeywords ───` section comment above them.

- [ ] **Step 2: Update `fakeScorer` and delete the 4 rate-limit tests**

In `LLMRateLimitService_test.go`: remove the `keywordsOut *dto.ResumeKeywordResult` field and `ExtractResumeKeywords` method from the `fakeScorer` struct (it still needs `ScoreBatch`/`EnrichBatch` — keep those). Delete: `TestRateLimit_ExtractResumeKeywords_Success`, `TestRateLimit_ExtractResumeKeywords_DenyLocalQuota`, `TestRateLimit_ExtractResumeKeywords_ReleaseOnInnerError`, `TestRateLimit_ExtractResumeKeywords_KeepReservationOnProvider429`.

- [ ] **Step 3: Update `fakeScorer` and delete the 8 router tests**

In `LLMRouterService_test.go`: remove the same `keywordsOut`/`ExtractResumeKeywords` pieces from its own `fakeScorer` (separate file, separate struct definition — confirm both files' `fakeScorer` are distinct, not shared, before editing). Delete: `TestRouter_ExtractResumeKeywords_FallsBackOnLocalQuota`, `TestRouter_ExtractResumeKeywords_RecordsCooldownOnProvider429`, `TestRouter_ExtractResumeKeywords_BubblesOtherErrors`, `TestRouter_ExtractResumeKeywords_ValidationErrorFailsFast`, `TestRouter_ExtractResumeKeywords_ReturnsAllExhaustedError`, `TestRouter_ExtractResumeKeywords_ZeroSlotsReturnsAllExhausted`.

- [ ] **Step 4: Run the full LLM test package**

Run: `cd backend && go test ./service/llm/... -v -race 2>&1 | tail -100`
Expected: PASS — everything remaining green, race-clean.

- [ ] **Step 5: Commit**

```bash
git add backend/service/llm/services/LLMGeminiService_http_internal_test.go \
        backend/service/llm/services/tests/LLMRateLimitService_test.go \
        backend/service/llm/services/tests/LLMRouterService_test.go
git commit -m "test(llm): remove ResumeKeywordExtractor tests"
```

---

## Part F — Wire cleanup

### Task F1: Update `resume/wire.go` — drop `NewKeywordStore`

**Files:**

- Modify: `backend/service/resume/wire.go`

- [ ] **Step 1: Update the provider set**

```go
package resume

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/resume/services"
)

var ProviderSet = wire.NewSet(
	services.NewRepositoryService,
	services.NewImplService,
	services.NewValidationService,
	services.NewRestService,
	services.NewRestRouter,
)
```

- [ ] **Step 2: Commit**

```bash
git add backend/service/resume/wire.go
git commit -m "feat(resume): drop NewKeywordStore from the wire provider set"
```

### Task F2: Update `apps/api/wire.go` — drop `provideKeywordExtractor` and its dependencies

**Files:**

- Modify: `backend/apps/api/wire.go`

- [ ] **Step 1: Remove `provideKeywordExtractor` from `wire.Build(...)`**

```go
func InitializeApp(db *gorm.DB, cfg *config.Config, log logger.Logger) (*App, error) {
	wire.Build(
		provideScanConfig,
		provideScanLogReader,
		provideScanHistoryReader,
		provideSeenLedgerWriter,
		provideJobApplicationWriter,
		provideObjectStorage,
		company.ProviderSet,
		scan.ProviderSet,
		job.ProviderSet,
		application.ProviderSet,
		resume.ProviderSet,
		applyqueue.ProviderSet,
		NewApp,
	)
	return nil, nil
}
```

- [ ] **Step 2: Delete `noopKeywordExtractor`, `buildLLMQuotaStores`, and `provideKeywordExtractor` entirely**

Delete everything from the `// noopKeywordExtractor is the resume slice's KeywordExtractor...` comment through the end of `provideKeywordExtractor`'s closing brace (the full block shown in research, roughly lines 92-150).

- [ ] **Step 3: Remove now-unused imports**

Remove `"fmt"`, `"time"`, `commonredis "github.com/trushilpatel/jobradar/common/redis"`, `llmdto "github.com/trushilpatel/jobradar/service/llm/dto"`, `llmcore "github.com/trushilpatel/jobradar/service/llm/services"`, `llmstore "github.com/trushilpatel/jobradar/service/llm/store"` from the import block — confirmed unused elsewhere in this file (only `"context"` survives, used by `provideObjectStorage`'s `context.Background()`).

- [ ] **Step 4: Regenerate `wire_gen.go`**

Run: `cd backend && go generate ./apps/api/`
Expected: `wire_gen.go` regenerates without error, with `resumeImplService := services4.NewImplService(resumeRepositoryService, objectStorage, log)` (3 args) and no `provideKeywordExtractor`/`noopKeywordExtractor`/`buildLLMQuotaStores` remaining.

- [ ] **Step 5: Build the whole backend**

Run: `cd backend && go build ./... 2>&1 | head -80`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/apps/api/wire.go backend/apps/api/wire_gen.go
git commit -m "feat(api): drop LLM-backed keyword extractor wiring from apps/api"
```

### Task F3: Run the full backend test suite to confirm Parts D–F land cleanly together

- [ ] **Step 1: Run everything**

Run: `cd backend && go build ./... && go vet ./... && go test -race -short ./... 2>&1 | tail -150`
Expected: PASS across the board.

- [ ] **Step 2: Run lint**

Run: `cd backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m`
Expected: 0 issues.

- [ ] **Step 3: If anything fails, fix inline and re-run before proceeding to Part G**

---

## Part G — Apply Queue backend: serve `JobKeywords`, drop matching

### Task G1: Replace `RecommendedResume*` with `JobKeywords` on `QueueItem`

**Files:**

- Modify: `backend/service/applyqueue/dto/QueueItem.go`

- [ ] **Step 1: Update the struct**

```go
// Package dto holds input/output shapes for the apply-queue service that
// are not GORM models.
package dto

import (
	"time"

	"gorm.io/datatypes"
)

// QueueItem is one row in the apply queue — a job the user is actively
// engaging with (saved and/or advanced to applied/interviewing), plus any
// resume currently attached to it.
//
// Status is derived, never a raw passthrough of user_job_state.status:
// NULL status (only saved_at set) renders as "saved"
// (jobcommon.JobStatusSaved); otherwise the actual status verbatim. Only
// {saved, applied, interviewing} are ever returned — both List filters
// exclude terminal statuses (dismissed/offer/rejected) even when saved_at
// is still set — matching the frontend's ApplyQueueStatusSchema
// three-value union.
//
// JSON field names for Title/URL are job_title/job_url (not the bare
// title/url used elsewhere, e.g. dto.JobView) — matches the already-shipped
// frontend contract in frontend/src/features/apply-queue/api/apply-queue.api.ts.
//
// JobKeywords is the job's canonical (techtag.Clean-cleaned) job_tech_tags,
// attached on-read per page (see loadJobTechTags in
// ApplyQueueRepositoryService.go) — plain data, no scoring. The frontend
// computes any resume-match against it client-side (see
// docs/superpowers/specs/2026-07-05-resume-job-keyword-match-design.md);
// the backend no longer recommends a resume.
//
// Location/Remote/SalaryRange/AddedAt are a verbatim pass-through of
// jobmodel.Job's fields of the same name — same JSON tags, same Go types —
// so the apply-queue row can render the same summary info as the Jobs page
// and back a detail panel, unlike Title/URL above which deliberately diverge
// from dto.JobView's naming. Location and SalaryRange are nullable at the DB
// level (a job may have no parsed location or salary) and surface as nil /
// JSON null respectively rather than panicking or omitting the field.
type QueueItem struct {
	JobID        string         `json:"job_id"`
	Title        string         `json:"job_title"`
	Company      string         `json:"company"`
	URL          string         `json:"job_url"`
	Status       string         `json:"status"`
	ResumeID     *string        `json:"resume_id"`
	ResumeLabel  *string        `json:"resume_label"`
	JobKeywords  []string       `json:"job_keywords"`
	Location     *string        `json:"location"`
	Remote       bool           `json:"remote"`
	SalaryRange  datatypes.JSON `json:"salary_range"`
	AddedAt      time.Time      `json:"added_at"`
}

// QueueListResult is the keyset-pagination response shape for
// GET /api/apply-queue.
type QueueListResult struct {
	Items      []QueueItem `json:"items"`
	NextCursor string      `json:"next_cursor"`
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/service/applyqueue/dto/QueueItem.go
git commit -m "feat(applyqueue): replace RecommendedResume fields with JobKeywords on QueueItem"
```

### Task G2: Delete `ApplyQueueMatching.go`, move `loadJobTechTags` into the repository, add `attachJobKeywords`

**Files:**

- Modify: `backend/service/applyqueue/services/ApplyQueueRepositoryService.go`
- Delete: `backend/service/applyqueue/services/ApplyQueueMatching.go`

- [ ] **Step 1: Replace `attachResumeRecommendations` with `attachJobKeywords` in `ApplyQueueRepositoryService.go`, and inline `loadJobTechTags`**

Replace the `List` method's item-construction loop and the `attachResumeRecommendations` method:

```go
	items := make([]dto.QueueItem, len(rows))
	for i, row := range rows {
		items[i] = dto.QueueItem{
			JobID:       row.JobID,
			Title:       row.Title,
			Company:     row.Company,
			URL:         row.URL,
			Status:      deriveStatus(row.Status),
			ResumeID:    row.ResumeID,
			ResumeLabel: row.ResumeLabel,
			Location:    row.Location,
			Remote:      row.Remote,
			SalaryRange: row.SalaryRange,
			AddedAt:     row.AddedAt,
		}
	}

	if len(items) > 0 {
		if kwErr := r.attachJobKeywords(ctx, items); kwErr != nil {
			return dto.QueueListResult{}, domainerrs.ErrInternal.WithMessage("applyqueue repo: job keywords: " + kwErr.Error())
		}
	}

	return dto.QueueListResult{Items: items, NextCursor: next}, nil
}

// jobTechTagRow is the raw scan target for a job_tech_tags lookup — one row
// per (job, tech) pair, matching the table's composite primary key.
type jobTechTagRow struct {
	JobID string `gorm:"column:job_id"`
	Tech  string `gorm:"column:tech"`
}

// attachJobKeywords fetches the canonical (techtag.Clean-cleaned at insert
// time — see JobPipelineImplService.go) job_tech_tags for every job on the
// page in a single query and attaches them to each item in place — one query
// per page, never one per row. Deliberately reads job_tech_tags rather than
// the jobs.tech_stack JSONB column, which is uncleaned raw LLM output per
// that table's own doc comment. Plain data, no scoring: matching against a
// resume happens client-side.
func (r *repositoryService) attachJobKeywords(ctx context.Context, items []dto.QueueItem) (err error) {
	ctx, span := tracing.Start(ctx, "applyqueue.repo.attachJobKeywords")
	defer tracing.End(span, &err)

	jobIDs := make([]string, len(items))
	for i, it := range items {
		jobIDs[i] = it.JobID
	}

	var rows []jobTechTagRow
	if dbErr := r.db.WithContext(ctx).Table("job_tech_tags").
		Select("job_id, tech").
		Where("job_id IN ?", jobIDs).
		Find(&rows).Error; dbErr != nil {
		return dbErr
	}

	byJob := make(map[string][]string, len(jobIDs))
	for _, row := range rows {
		byJob[row.JobID] = append(byJob[row.JobID], row.Tech)
	}
	for i := range items {
		items[i].JobKeywords = byJob[items[i].JobID]
	}
	return nil
}
```

- [ ] **Step 2: Delete `ApplyQueueMatching.go`**

```bash
rm backend/service/applyqueue/services/ApplyQueueMatching.go
```

- [ ] **Step 3: Build**

Run: `cd backend && go build ./service/applyqueue/... 2>&1 | head -40`
Expected: FAIL only in tests (Task G3) — the `services` package itself should compile.

- [ ] **Step 4: Commit**

```bash
git add backend/service/applyqueue/services/ApplyQueueRepositoryService.go
git rm backend/service/applyqueue/services/ApplyQueueMatching.go
git commit -m "feat(applyqueue): replace Jaccard resume matching with plain JobKeywords attachment"
```

### Task G3: Update applyqueue tests

**Files:**

- Modify: `backend/service/applyqueue/tests/repotestdb_test.go`
- Modify: `backend/service/applyqueue/tests/ApplyQueueRepositoryService_test.go`
- Modify: `backend/service/applyqueue/tests/ApplyQueueRepository_dberr_test.go`

- [ ] **Step 1: Update the hand-written `resumes` schema in `repotestdb_test.go`**

```go
		`CREATE TABLE resumes (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			label TEXT NOT NULL,
			original_filename TEXT NOT NULL,
			storage_key TEXT NOT NULL UNIQUE,
			content_type TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			extracted_text TEXT NOT NULL DEFAULT ''
		)`,
```

- [ ] **Step 2: Delete `seedResumeWithKeywords`**

Delete the function and its `"encoding/json"` import if nothing else in the file uses `json.` (check first).

- [ ] **Step 3: Delete all 9 `TestApplyQueueRepo_List_Recommendation_*` tests from `ApplyQueueRepositoryService_test.go`**

Delete: `TestApplyQueueRepo_List_Recommendation_ExactMatchScoresOne`, `TestApplyQueueRepo_List_Recommendation_PicksHighestScoringResumeAmongMultiple`, `TestApplyQueueRepo_List_Recommendation_NoJobTechTagsIsNil`, `TestApplyQueueRepo_List_Recommendation_ResumeWithEmptyKeywordsNeverRecommended`, `TestApplyQueueRepo_List_Recommendation_DisjointKeywordsScoreZeroIsNil`, `TestApplyQueueRepo_List_Recommendation_TieBreaksToOldestResume`, `TestApplyQueueRepo_List_Recommendation_IndependentPerJobOnSamePage`, `TestApplyQueueRepo_List_Recommendation_NoResumesAtAllIsNil`, `TestApplyQueueRepo_List_Recommendation_ScopedToCallerResumesOnly` (and the `// ─── Resume recommendation (issue #264) ──` section header above them).

- [ ] **Step 4: Add tests for `JobKeywords`**

```go
// ─── Job keywords (replaces resume recommendation, issue #264 follow-up) ────

func TestApplyQueueRepo_List_JobKeywords_PopulatedFromTechTags(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	seedTechTags(t, db, "j1", "c1", "Go", "PostgreSQL")
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(result.Items))
	}
	got := result.Items[0].JobKeywords
	if len(got) != 2 || got[0] != "Go" || got[1] != "PostgreSQL" {
		t.Errorf("JobKeywords = %v, want [Go PostgreSQL]", got)
	}
}

func TestApplyQueueRepo_List_JobKeywords_NoTechTagsIsEmpty(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer") // no seedTechTags call
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items[0].JobKeywords) != 0 {
		t.Errorf("expected empty JobKeywords for a job with no tech tags, got %v", result.Items[0].JobKeywords)
	}
}

func TestApplyQueueRepo_List_JobKeywords_IndependentPerJobOnSamePage(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Backend Engineer")
	seedJob(t, db, "j2", "c1", "Frontend Engineer")
	seedTechTags(t, db, "j1", "c1", "Go", "PostgreSQL")
	seedTechTags(t, db, "j2", "c1", "React", "TypeScript")
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)
	seedUserJobState(t, db, testUserID, "j2", nil, &savedAt, now.Add(time.Second))

	repo := applyqueuesvc.NewRepositoryService(db)
	result, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20)
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(result.Items))
	}

	byJobID := map[string][]string{}
	for _, item := range result.Items {
		byJobID[item.JobID] = item.JobKeywords
	}
	if len(byJobID["j1"]) != 2 || byJobID["j1"][0] != "Go" {
		t.Errorf("j1 JobKeywords = %v, want [Go PostgreSQL]", byJobID["j1"])
	}
	if len(byJobID["j2"]) != 2 || byJobID["j2"][0] != "React" {
		t.Errorf("j2 JobKeywords = %v, want [React TypeScript]", byJobID["j2"])
	}
}
```

- [ ] **Step 5: Update `ApplyQueueRepository_dberr_test.go`**

Rename `TestApplyQueueRepo_List_RecommendationTechTagsQueryDBError` to `TestApplyQueueRepo_List_JobKeywordsQueryDBError` and update its doc comment:

```go
// TestApplyQueueRepo_List_JobKeywordsQueryDBError forces attachJobKeywords'
// job_tech_tags lookup to fail (base query already succeeded — there's a row
// to attach keywords to) and asserts List surfaces it as an error rather than
// silently returning items with no keywords.
func TestApplyQueueRepo_List_JobKeywordsQueryDBError(t *testing.T) {
	t.Parallel()
	db := newApplyQueueTestDB(t)
	seedCompany(t, db, "c1", "Acme")
	seedJob(t, db, "j1", "c1", "Engineer")
	now := time.Now()
	savedAt := now
	seedUserJobState(t, db, testUserID, "j1", nil, &savedAt, now)

	if err := db.Exec("DROP TABLE job_tech_tags").Error; err != nil {
		t.Fatalf("drop job_tech_tags: %v", err)
	}

	repo := applyqueuesvc.NewRepositoryService(db)
	if _, err := repo.List(context.Background(), testUserID, applyqueuecommon.FilterAll, "", 20); err == nil {
		t.Fatal("expected an error when the job_tech_tags lookup fails")
	}
}
```

Delete `TestApplyQueueRepo_List_RecommendationResumesQueryDBError` entirely (no resumes-lookup exists in `attachJobKeywords`).

- [ ] **Step 6: Run the applyqueue test package**

Run: `cd backend && go test ./service/applyqueue/... -v -race 2>&1 | tail -100`
Expected: PASS — all remaining/new tests green, race-clean.

- [ ] **Step 7: Commit**

```bash
git add backend/service/applyqueue/tests/repotestdb_test.go \
        backend/service/applyqueue/tests/ApplyQueueRepositoryService_test.go \
        backend/service/applyqueue/tests/ApplyQueueRepository_dberr_test.go
git commit -m "test(applyqueue): replace resume-recommendation tests with JobKeywords tests"
```

### Task G4: Full backend verification

- [ ] **Step 1: Build, vet, race test, lint, coverage**

Run:

```bash
cd backend
go build ./...
go vet ./...
go test -race -short ./...
golangci-lint run --config ../.github/.golangci.yml --timeout 5m
scripts/../../scripts/check-coverage.sh /tmp/cov.out 95 2>/dev/null || (go test ./... -coverprofile=/tmp/cov.out && ../scripts/check-coverage.sh /tmp/cov.out 95)
```

Expected: everything PASS, coverage ≥95% on every touched package (`service/resume/...`, `service/llm/...`, `service/applyqueue/...`, `service/job/techtag`).

- [ ] **Step 2: If coverage is short on any touched package, add tests before proceeding — do not lower the bar**

- [ ] **Step 3: This is a natural checkpoint — do not proceed to Part H (frontend) until this is fully green**

---

## Part H — Frontend: resumes feature cleanup

### Task H1: Add scoped cache-TTL config

**Files:**

- Create: `frontend/src/lib/core/cacheConfig.ts`
- Test: `frontend/src/lib/core/__tests__/cacheConfig.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect } from 'vitest'
import { CACHE_TTL } from '../cacheConfig'

describe('CACHE_TTL', () => {
  it('defines a longer staleTime/gcTime for resumes than the global default', () => {
    expect(CACHE_TTL.resumes.staleTime).toBe(5 * 60_000)
    expect(CACHE_TTL.resumes.gcTime).toBe(30 * 60_000)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/lib/core/__tests__/cacheConfig.test.ts`
Expected: FAIL — module `../cacheConfig` does not exist.

- [ ] **Step 3: Create `cacheConfig.ts`**

```ts
// Central query-cache TTL config — scoped so far to queries known to be safe
// with a longer cache window (data that changes rarely and is already
// explicitly invalidated on the mutations that do change it). Queries that
// must stay "live" (e.g. useApplyQueue — job status/resume attachment must
// read as current on every navigation) are deliberately NOT added here; they
// keep their own inline staleTime/gcTime. Adding a new domain to this cache
// tier is one entry here plus pointing its hook at it — no change to the
// mechanism itself.
export const CACHE_TTL = {
  // Resumes rarely change — only on upload/delete, both of which already
  // invalidate resumeQueryKeys.list() explicitly (see useResumeActions.ts).
  resumes: { staleTime: 5 * 60_000, gcTime: 30 * 60_000 },
} as const
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && npx vitest run src/lib/core/__tests__/cacheConfig.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/core/cacheConfig.ts frontend/src/lib/core/__tests__/cacheConfig.test.ts
git commit -m "feat(lib): add scoped CACHE_TTL config, starting with resumes"
```

### Task H2: Point `useResumes` at `CACHE_TTL.resumes`

**Files:**

- Modify: `frontend/src/features/resumes/hooks/useResumes.ts`
- Modify: `frontend/src/features/resumes/hooks/__tests__/useResumes.test.ts`

- [ ] **Step 1: Update the hook**

```ts
import { useQuery } from '@tanstack/react-query'
import { resumeQueryKeys } from '../queryKeys'
import { fetchResumes } from '../api/resumes.api'
import { CACHE_TTL } from '@/lib/core/cacheConfig'

export function useResumes() {
  return useQuery({
    queryKey: resumeQueryKeys.list(),
    queryFn: () => fetchResumes(),
    staleTime: CACHE_TTL.resumes.staleTime,
    gcTime: CACHE_TTL.resumes.gcTime,
  })
}
```

- [ ] **Step 2: Check the existing test file for a hard-coded `30_000` assertion and update it**

Run: `grep -n "30_000\|staleTime\|gcTime" frontend/src/features/resumes/hooks/__tests__/useResumes.test.ts`. If a test asserts the query's `staleTime` option directly (unlikely for a `renderHook` test, which typically asserts on returned `data`/`isLoading`, not internal query options), update it to import `CACHE_TTL` and assert against `CACHE_TTL.resumes.staleTime`/`gcTime` instead of a bare literal. If no such assertion exists, no change needed beyond confirming the test still passes.

- [ ] **Step 3: Run the test**

Run: `cd frontend && npx vitest run src/features/resumes/hooks/__tests__/useResumes.test.ts`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/resumes/hooks/useResumes.ts frontend/src/features/resumes/hooks/__tests__/useResumes.test.ts
git commit -m "feat(resumes): use CACHE_TTL.resumes instead of an inline staleTime literal"
```

### Task H3: Replace `keywords` with `extracted_text` in the resumes feature types/API

**Files:**

- Modify: `frontend/src/features/resumes/types.ts`
- Modify: `frontend/src/features/resumes/api/resumes.api.ts`
- Modify: `frontend/src/features/resumes/api/__tests__/resumes.api.test.ts`

- [ ] **Step 1: Update `types.ts`**

```ts
// Resume feature types — mirrors the flat `resumes` table (migration 086,
// extracted_text added in 090). No versioning/tags/attachment fields: those
// belong to the larger, not-yet-built feature in
// docs/design/resume-feature-spec.md.

export interface Resume {
  id: string
  user_id: string
  label: string
  original_filename: string
  content_type:
    | 'application/pdf'
    | 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'
  size_bytes: number
  created_at: string
  // Plain text extracted at upload (no LLM) — internal matching input only,
  // never rendered directly. See
  // docs/superpowers/specs/2026-07-05-resume-job-keyword-match-design.md.
  extracted_text: string
}

export const RESUME_LIMIT_PER_USER = 50
export const RESUME_MAX_FILE_BYTES = 2 * 1024 * 1024
export const RESUME_ALLOWED_EXTENSIONS = ['.pdf', '.docx'] as const
```

(`RESUME_KEYWORDS_VISIBLE_LIMIT` is removed — no more keyword badges to cap.)

- [ ] **Step 2: Update `resumes.api.ts`**

```ts
import { z } from 'zod'
import { apiClient } from '@/lib/core/api-client'
import type { Resume } from '../types'

const ResumeSchema = z.object({
  id: z.string(),
  user_id: z.string(),
  label: z.string(),
  original_filename: z.string(),
  content_type: z.union([
    z.literal('application/pdf'),
    z.literal('application/vnd.openxmlformats-officedocument.wordprocessingml.document'),
  ]),
  size_bytes: z.number(),
  created_at: z.string(),
  // Backend always sends a string (possibly empty); `.catch('')` is a
  // defensive normalizer in case an older/drifted response omits the field.
  extracted_text: z.string().catch(''),
})
const ResumeListSchema = z.array(ResumeSchema)

export async function fetchResumes(): Promise<Resume[]> {
  const raw = await apiClient.get<unknown>('/resumes')
  return ResumeListSchema.parse(raw) as Resume[]
}

export async function uploadResume(file: File, label: string): Promise<Resume> {
  const form = new FormData()
  form.append('file', file, file.name)
  if (label) form.append('label', label)
  const raw = await apiClient.upload<unknown>('/resumes', form)
  return ResumeSchema.parse(raw) as Resume
}

export async function downloadResume(id: string): Promise<Blob> {
  return apiClient.getBlob(`/resumes/${encodeURIComponent(id)}/download`)
}

export async function renameResume(id: string, label: string): Promise<Resume> {
  const raw = await apiClient.patch<unknown>(`/resumes/${encodeURIComponent(id)}`, { label })
  return ResumeSchema.parse(raw) as Resume
}

export async function deleteResume(id: string): Promise<void> {
  await apiClient.delete<void>(`/resumes/${encodeURIComponent(id)}`)
}
```

(`extractResumeKeywords` function is removed entirely.)

- [ ] **Step 3: Update `resumes.api.test.ts`**

Run: `grep -n "keywords\|extractResumeKeywords" frontend/src/features/resumes/api/__tests__/resumes.api.test.ts` and update every fixture object's `keywords: [...]` field to `extracted_text: '...'` (a string, not an array), and delete any `describe`/`it` blocks that test `extractResumeKeywords`.

- [ ] **Step 4: Run the test**

Run: `cd frontend && npx vitest run src/features/resumes/api/__tests__/resumes.api.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/resumes/types.ts frontend/src/features/resumes/api/resumes.api.ts \
        frontend/src/features/resumes/api/__tests__/resumes.api.test.ts
git commit -m "feat(resumes): replace keywords with extracted_text in types/API"
```

### Task H4: Delete `useExtractResumeKeywords`

**Files:**

- Delete: `frontend/src/features/resumes/hooks/useExtractResumeKeywords.ts`
- Delete: `frontend/src/features/resumes/hooks/__tests__/useExtractResumeKeywords.test.ts`
- Modify: `frontend/src/features/resumes/index.ts`

- [ ] **Step 1: Delete the files**

```bash
rm frontend/src/features/resumes/hooks/useExtractResumeKeywords.ts \
   frontend/src/features/resumes/hooks/__tests__/useExtractResumeKeywords.test.ts
```

- [ ] **Step 2: Update the barrel**

```ts
export { useResumes } from './hooks/useResumes'
export { useResumeActions } from './hooks/useResumeActions'
export { ResumeCard } from './components/ResumeCard'
export { ResumeStorageMeter } from './components/ResumeStorageMeter'
export { UploadResumeDialog } from './components/UploadResumeDialog'
export type { Resume } from './types'
export { RESUME_LIMIT_PER_USER, RESUME_MAX_FILE_BYTES, RESUME_ALLOWED_EXTENSIONS } from './types'
export { resumeQueryKeys } from './queryKeys'
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/resumes/index.ts
git rm frontend/src/features/resumes/hooks/useExtractResumeKeywords.ts \
       frontend/src/features/resumes/hooks/__tests__/useExtractResumeKeywords.test.ts
git commit -m "feat(resumes): remove useExtractResumeKeywords hook"
```

### Task H5: Simplify `ResumeCard.tsx`

**Files:**

- Modify: `frontend/src/features/resumes/components/ResumeCard.tsx`
- Modify: `frontend/src/features/resumes/components/__tests__/ResumeCard.test.tsx`

- [ ] **Step 1: Write/update the failing test first**

Update `ResumeCard.test.tsx`: remove any test asserting keyword badges render or that the "Extract keywords" button exists/calls a mutation. Add:

```tsx
it('does not render keyword badges or an extract-keywords action', () => {
  render(
    <ResumeCard
      resume={{
        id: 'r1',
        user_id: 'u1',
        label: 'My resume',
        original_filename: 'resume.pdf',
        content_type: 'application/pdf',
        size_bytes: 2048,
        created_at: '2026-07-03T00:00:00Z',
        extracted_text: 'Backend engineer with Go experience.',
      }}
    />
  )
  expect(screen.queryByLabelText('Extract keywords')).not.toBeInTheDocument()
})
```

(Adjust the fixture/imports to match whatever mocking pattern the existing test file already uses for `useResumeActions` — mirror the existing test setup exactly rather than introducing a new pattern.)

- [ ] **Step 2: Run to verify it fails (or passes trivially before the component change — since the assertion is "not in document", it may pass even before the edit if the button already isn't there under some condition; the meaningful check is that no test elsewhere still asserts the button DOES appear)**

Run: `cd frontend && npx vitest run src/features/resumes/components/__tests__/ResumeCard.test.tsx`
Expected: existing tests that assert the "Extract keywords" button appears (when `keywords.length === 0`) FAIL to compile/run once `Resume.extracted_text` replaces `Resume.keywords` in the type (Task H3 already landed) — this is the forcing function. Delete those obsolete assertions now.

- [ ] **Step 3: Simplify `ResumeCard.tsx`**

```tsx
import { useState } from 'react'
import { FileText, Download, Trash2, Pencil } from 'lucide-react'
import { cn, formatRelativeTime } from '@/lib/utils/utils'
import { Input } from '@/components/ui/input'
import { useResumeActions } from '../hooks/useResumeActions'
import type { Resume } from '../types'

// Triggers a browser "save as" for an in-memory Blob via a throwaway <a
// download> element — the standard DOM pattern for saving fetched bytes
// without a server redirect. The object URL is revoked on the next tick
// (not synchronously) since some browsers cancel the save if revoked before
// they've had a chance to start reading it.
function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 0)
}

export function ResumeCard({ resume }: { resume: Resume }) {
  const { download, rename, remove } = useResumeActions()
  const [editing, setEditing] = useState(false)
  const [label, setLabel] = useState(resume.label)

  async function handleDownload() {
    const blob = await download.mutateAsync(resume.id)
    saveBlob(blob, resume.original_filename)
  }

  async function handleRenameSubmit() {
    if (label.trim() && label !== resume.label) {
      await rename.mutateAsync({ id: resume.id, label: label.trim() })
    }
    setEditing(false)
  }

  return (
    <div className={cn('rounded-xl border border-[var(--border)] bg-panel p-4')}>
      <div className="flex items-start justify-between gap-3 mb-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-primary font-medium truncate">
            <FileText size={14} className="text-muted shrink-0" />
            {editing ? (
              <Input
                autoFocus
                value={label}
                onChange={(e) => setLabel(e.target.value)}
                onBlur={handleRenameSubmit}
                onKeyDown={(e) => e.key === 'Enter' && handleRenameSubmit()}
                className="h-6 text-sm"
              />
            ) : (
              <span className="truncate">{resume.label}</span>
            )}
          </div>
          <div className="text-[11px] text-muted mt-0.5 font-mono truncate">
            {resume.original_filename}
          </div>
        </div>
      </div>

      <div className="flex items-center justify-between text-xs text-muted">
        <span>
          {(resume.size_bytes / 1024).toFixed(0)} KB · Uploaded{' '}
          {formatRelativeTime(resume.created_at)}
        </span>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setEditing(true)}
            aria-label="Rename"
            className="hover:text-primary"
          >
            <Pencil size={13} />
          </button>
          <button
            type="button"
            onClick={handleDownload}
            aria-label="Download"
            className="hover:text-primary"
          >
            <Download size={13} />
          </button>
          <button
            type="button"
            onClick={() => remove.mutate(resume.id)}
            aria-label="Delete"
            className="hover:text-red-400"
          >
            <Trash2 size={13} />
          </button>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Run the test**

Run: `cd frontend && npx vitest run src/features/resumes/components/__tests__/ResumeCard.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/resumes/components/ResumeCard.tsx frontend/src/features/resumes/components/__tests__/ResumeCard.test.tsx
git commit -m "feat(resumes): remove keyword badges and extract-keywords action from ResumeCard"
```

### Task H6: Update `pages/__tests__/ApplyQueuePage.test.tsx`'s `sampleResume` fixture (partial — full page rewiring is Part I)

- [ ] **Step 1: Note only — do not edit yet**

`ApplyQueuePage.test.tsx`'s `sampleResume` fixture (`keywords: [] as string[]`) and `sampleItem` fixture (`recommended_resume_id`/`recommended_resume_label`) both need updating, but that file also needs the Part I frontend changes (job_keywords, match column) landed first to avoid editing it twice. This is deferred to Task I8.

---

## Part I — Frontend: apply-queue feature + matching utility + page wiring

### Task I1: Write the `resumeJobMatch` matching utility

**Files:**

- Create: `frontend/src/lib/utils/resumeJobMatch.ts`
- Test: `frontend/src/lib/utils/__tests__/resumeJobMatch.test.ts`

- [ ] **Step 1: Write the failing tests**

```ts
import { describe, it, expect } from 'vitest'
import { matchResume, pickBestResume } from '../resumeJobMatch'

describe('matchResume', () => {
  it('counts case-insensitive whole-word matches', () => {
    const result = matchResume(
      ['Go', 'PostgreSQL'],
      'Experienced backend engineer using go and PostgreSQL daily.'
    )
    expect(result).toEqual({ matched: 2, total: 2 })
  })

  it('does not match a keyword as a substring of a longer word', () => {
    const result = matchResume(['Go'], 'I use Google Cloud extensively.')
    expect(result).toEqual({ matched: 0, total: 1 })
  })

  it('matches multi-word keywords as a whole phrase', () => {
    const result = matchResume(['machine learning'], 'Background in machine learning and NLP.')
    expect(result).toEqual({ matched: 1, total: 1 })
  })

  it('correctly escapes regex special characters in keywords like C++ and Node.js', () => {
    const result = matchResume(
      ['C++', 'Node.js'],
      'Built services in C++ and Node.js for five years.'
    )
    expect(result).toEqual({ matched: 2, total: 2 })
  })

  it('returns total 0 for an empty keyword list', () => {
    expect(matchResume([], 'some resume text')).toEqual({ matched: 0, total: 0 })
  })

  it('returns matched 0 for empty resume text', () => {
    expect(matchResume(['Go'], '')).toEqual({ matched: 0, total: 1 })
  })
})

describe('pickBestResume', () => {
  const resumes = [
    { id: 'r1', label: 'Weak', extractedText: 'Go and Python developer.' },
    { id: 'r2', label: 'Strong', extractedText: 'Go, PostgreSQL, and Kubernetes expert.' },
  ]

  it('picks the resume with the highest match count', () => {
    const best = pickBestResume(['Go', 'PostgreSQL', 'Kubernetes'], resumes)
    expect(best).toEqual({ id: 'r2', label: 'Strong' })
  })

  it('returns null when there are no job keywords', () => {
    expect(pickBestResume([], resumes)).toBeNull()
  })

  it('returns null when there are no resumes', () => {
    expect(pickBestResume(['Go'], [])).toBeNull()
  })

  it('returns null when every resume scores zero', () => {
    expect(pickBestResume(['Rust'], resumes)).toBeNull()
  })

  it('breaks ties by keeping the first-encountered (oldest-uploaded) resume', () => {
    const tied = [
      { id: 'r-first', label: 'First', extractedText: 'Go and PostgreSQL.' },
      { id: 'r-second', label: 'Second', extractedText: 'Go and PostgreSQL.' },
    ]
    expect(pickBestResume(['Go', 'PostgreSQL'], tied)).toEqual({ id: 'r-first', label: 'First' })
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd frontend && npx vitest run src/lib/utils/__tests__/resumeJobMatch.test.ts`
Expected: FAIL — module `../resumeJobMatch` does not exist.

- [ ] **Step 3: Implement `resumeJobMatch.ts`**

```ts
// Pure, LLM-free matching between a job's canonical keywords and a resume's
// extracted plain text — see
// docs/superpowers/specs/2026-07-05-resume-job-keyword-match-design.md.
// Lives in lib/ (not a feature) because it is used by the page layer that
// composes features/apply-queue and features/resumes, which may not import
// each other directly.

// escapeRegExp escapes regex-special characters in a keyword so it can be
// safely embedded in a RegExp source string — required for keywords like
// "C++", "C#", and "Node.js" that contain characters with regex meaning.
function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// keywordRegExp builds a case-insensitive, whole-word-boundary matcher for a
// single keyword. \b works correctly around word characters on either side
// of a multi-word phrase (e.g. "machine learning") since \b matches at the
// transition between a word char and a non-word char (including a plain
// space), not just around single tokens.
function keywordRegExp(keyword: string): RegExp {
  return new RegExp(`\\b${escapeRegExp(keyword)}\\b`, 'i')
}

export interface MatchResult {
  matched: number
  total: number
}

// matchResume counts how many of jobKeywords appear as whole-word,
// case-insensitive matches in resumeText.
export function matchResume(jobKeywords: string[], resumeText: string): MatchResult {
  const total = jobKeywords.length
  if (total === 0) return { matched: 0, total: 0 }

  let matched = 0
  for (const keyword of jobKeywords) {
    if (keywordRegExp(keyword).test(resumeText)) matched++
  }
  return { matched, total }
}

export interface ResumeForMatch {
  id: string
  label: string
  extractedText: string
}

export interface ResumeMatchPick {
  id: string
  label: string
}

// pickBestResume returns the resume with the highest matchResume score among
// resumes, or null when there are no job keywords, no resumes, or every
// resume scores zero (no recommendation is better than an arbitrary one).
// Ties keep the first-encountered resume — callers pass resumes pre-sorted
// oldest-first so this deterministically prefers the earliest upload,
// mirroring the backend's prior tie-break semantics.
export function pickBestResume(
  jobKeywords: string[],
  resumes: ResumeForMatch[]
): ResumeMatchPick | null {
  if (jobKeywords.length === 0 || resumes.length === 0) return null

  let bestScore = 0
  let best: ResumeMatchPick | null = null
  for (const resume of resumes) {
    const { matched } = matchResume(jobKeywords, resume.extractedText)
    if (matched > bestScore) {
      bestScore = matched
      best = { id: resume.id, label: resume.label }
    }
  }
  return best
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd frontend && npx vitest run src/lib/utils/__tests__/resumeJobMatch.test.ts`
Expected: PASS.

- [ ] **Step 5: Run lint + tsc on just this file's area**

Run: `cd frontend && npx eslint src/lib/utils/resumeJobMatch.ts src/lib/utils/__tests__/resumeJobMatch.test.ts`
Expected: 0 issues.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/utils/resumeJobMatch.ts frontend/src/lib/utils/__tests__/resumeJobMatch.test.ts
git commit -m "feat(lib): add word-boundary resume-job keyword matching utility"
```

### Task I2: Update `apply-queue.api.ts` — `job_keywords` replaces `recommended_resume_*`

**Files:**

- Modify: `frontend/src/features/apply-queue/api/apply-queue.api.ts`
- Modify: `frontend/src/features/apply-queue/api/__tests__/apply-queue.api.test.ts`

- [ ] **Step 1: Update the schema and type**

```ts
const ApplyQueueItemSchema = z.object({
  job_id: z.string(),
  job_title: z.string(),
  company: z.string(),
  job_url: z.string(),
  status: ApplyQueueStatusSchema,
  resume_id: z.string().nullable(),
  resume_label: z.string().nullable(),
  // Canonical job keywords (broadened past pure tech stack — see
  // docs/superpowers/specs/2026-07-05-resume-job-keyword-match-design.md).
  // The frontend computes any resume match against this client-side; the
  // backend performs no scoring.
  job_keywords: z.array(z.string()).catch([]),
  // Job info columns — same field names/shapes as the `Job` type's
  // equivalents (features/jobs/api/jobs.api.ts) so the Apply Queue table can
  // render the same location/salary/posted cells as the Jobs table.
  location: z.string().nullable(),
  remote: z.boolean(),
  salary_range: ApplyQueueSalaryRangeSchema,
  added_at: z.string(),
})
```

- [ ] **Step 2: Update the test fixtures**

Run: `grep -n "recommended_resume_id\|recommended_resume_label" frontend/src/features/apply-queue/api/__tests__/apply-queue.api.test.ts` and replace every fixture's `recommended_resume_id`/`recommended_resume_label` fields with `job_keywords: []` (or a populated array where the test is meaningfully exercising keyword data).

- [ ] **Step 3: Run the test**

Run: `cd frontend && npx vitest run src/features/apply-queue/api/__tests__/apply-queue.api.test.ts`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/apply-queue/api/apply-queue.api.ts frontend/src/features/apply-queue/api/__tests__/apply-queue.api.test.ts
git commit -m "feat(apply-queue): replace recommended_resume_* with job_keywords in the API contract"
```

### Task I3: Simplify `ResumePickerCell` — drop the `recommended` badge

**Files:**

- Modify: `frontend/src/features/apply-queue/components/ResumePickerCell.tsx`
- Modify: `frontend/src/features/apply-queue/components/__tests__/ResumePickerCell.test.tsx`

- [ ] **Step 1: Update the interface and remove badge rendering**

```tsx
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils/utils'

// Cross-feature note: this cell only ever receives plain { id, label } pairs,
// never the resumes feature's `Resume` domain type — importing across
// features is forbidden (see features/CLAUDE.md). The pages layer (which may
// import both features) is responsible for mapping `Resume[]` down to this
// shape, the same pattern JobsTable's `getAttachmentState` callback uses to
// avoid a features/jobs -> features/resumes import.
export interface ApplyQueueResumeOption {
  id: string
  label: string
}

interface ResumePickerCellProps {
  jobId: string
  resumeId: string | null
  resumeLabel: string | null
  options: ApplyQueueResumeOption[]
  onChange: (jobId: string, resumeId: string | null) => void
  disabled?: boolean
}

// Radix Select's Item value cannot be an empty string, so "no resume" is
// represented by this sentinel and translated back to `null` at the callback
// boundary — callers never see the sentinel.
const NONE_VALUE = '__none__'

export function ResumePickerCell({
  jobId,
  resumeId,
  resumeLabel,
  options,
  onChange,
  disabled,
}: ResumePickerCellProps) {
  // Defensive: the attached resume may have been deleted (or otherwise not be
  // in the caller's current options list) after being selected here. Without
  // this, Radix Select would render blank instead of the orphaned label,
  // silently hiding which resume was actually attached to the job.
  const attachedNotInOptions = resumeId !== null && !options.some((o) => o.id === resumeId)

  return (
    <Select
      value={resumeId ?? NONE_VALUE}
      onValueChange={(value) => onChange(jobId, value === NONE_VALUE ? null : value)}
      disabled={disabled}
    >
      <SelectTrigger
        aria-label={`Select resume for ${jobId}`}
        onClick={(e) => e.stopPropagation()}
        className={cn('h-7 w-full max-w-[220px] text-xs', disabled && 'opacity-60')}
      >
        <SelectValue placeholder="No resume selected" />
      </SelectTrigger>
      <SelectContent onClick={(e) => e.stopPropagation()}>
        <SelectItem value={NONE_VALUE}>
          <span className="text-placeholder">No resume selected</span>
        </SelectItem>
        {attachedNotInOptions && resumeId && (
          <SelectItem value={resumeId}>{resumeLabel ?? 'Unknown resume'}</SelectItem>
        )}
        {options.map((opt) => (
          <SelectItem key={opt.id} value={opt.id}>
            <span className="truncate">{opt.label}</span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
```

- [ ] **Step 2: Update the test file**

Run: `grep -n "recommended\|Recommended" frontend/src/features/apply-queue/components/__tests__/ResumePickerCell.test.tsx` and delete any test asserting the "Recommended" badge/star renders (that behavior moves to Task I6's auto-pick-as-selected-value, tested at the column/page level instead).

- [ ] **Step 3: Run the test**

Run: `cd frontend && npx vitest run src/features/apply-queue/components/__tests__/ResumePickerCell.test.tsx`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/apply-queue/components/ResumePickerCell.tsx frontend/src/features/apply-queue/components/__tests__/ResumePickerCell.test.tsx
git commit -m "feat(apply-queue): drop the Recommended badge from ResumePickerCell"
```

### Task I4: Add the match-summary column and effective-resume resolution to `apply-queue-columns.tsx`

**Files:**

- Modify: `frontend/src/features/apply-queue/components/apply-queue-columns.tsx`
- Modify: `frontend/src/features/apply-queue/components/__tests__/apply-queue-columns.test.tsx`

- [ ] **Step 1: Write/update the failing test first**

In `apply-queue-columns.test.tsx`, replace any test exercising the old `recommended` badge mapping with:

```tsx
it('renders the match summary for the row and passes the effective resume to the picker', () => {
  const columns = buildApplyQueueColumns({
    resumeOptions: [{ id: 'r1', label: 'My resume' }],
    onResumeChange: vi.fn(),
    getEffectiveResume: (jobId) => (jobId === 'job-1' ? { id: 'r1', label: 'My resume' } : null),
    getMatchSummary: (jobId) => (jobId === 'job-1' ? '3/5 matched' : null),
  })
  // ...render via the table/DataTable harness this file already uses, and
  // assert the "match" column cell for job-1 renders "3/5 matched" while the
  // resume picker cell shows resumeId="r1" — mirror the existing rendering
  // pattern already used in this test file for the "resume" column.
})

it('renders a placeholder when no match summary is available', () => {
  const columns = buildApplyQueueColumns({
    resumeOptions: [],
    onResumeChange: vi.fn(),
    getEffectiveResume: () => null,
    getMatchSummary: () => null,
  })
  // ...assert the match column cell renders the "—" placeholder for a row
  // with no effective resume, mirroring this file's existing cell-rendering
  // test pattern.
})
```

(Match the exact rendering/assertion harness this test file already uses elsewhere in the file — do not introduce a new one.)

- [ ] **Step 2: Run to verify failure**

Run: `cd frontend && npx vitest run src/features/apply-queue/components/__tests__/apply-queue-columns.test.tsx`
Expected: FAIL — `getEffectiveResume`/`getMatchSummary` don't exist on `ApplyQueueColumnActions` yet.

- [ ] **Step 3: Update `apply-queue-columns.tsx`**

```tsx
import type { ColumnDef } from '@tanstack/react-table'
import { DataTableColumnHeader } from '@/components/ui/data-table'
import { JobLocationCell } from '@/components/shared/JobLocationCell'
import { formatRelativeTime, formatExactDateTime, formatSalary } from '@/lib/utils/utils'
import type { ApplyQueueItem } from '../api/apply-queue.api'
import { ApplyQueueStatusPill } from './ApplyQueueStatusPill'
import { ResumePickerCell, type ApplyQueueResumeOption } from './ResumePickerCell'

export type { ApplyQueueResumeOption }

export interface EffectiveResume {
  id: string
  label: string
}

export interface ApplyQueueColumnActions {
  resumeOptions: ApplyQueueResumeOption[]
  onResumeChange: (jobId: string, resumeId: string | null) => void
  /** True while a resume-attach mutation for this job is in flight — disables its picker. */
  isResumeUpdatePending?: (jobId: string) => boolean
  /**
   * Resolves the resume actually shown as selected for a row — the user's
   * explicit attachment when one exists, otherwise the best client-side
   * auto-pick, otherwise null. Computed at the page layer (see
   * pages/apply-queue/useApplyQueueResumeMatch.ts) since it needs both this
   * feature's items and the resumes feature's data.
   */
  getEffectiveResume: (jobId: string) => EffectiveResume | null
  /** One-line "N/M matched" summary for the row's effective resume, or null when none is selected or the job has no keywords. */
  getMatchSummary: (jobId: string) => string | null
}

export function buildApplyQueueColumns(
  actions: ApplyQueueColumnActions
): ColumnDef<ApplyQueueItem, unknown>[] {
  const {
    resumeOptions,
    onResumeChange,
    isResumeUpdatePending,
    getEffectiveResume,
    getMatchSummary,
  } = actions

  return [
    // ── 1. Job (title, linked to posting + company) ────────────────────────
    {
      id: 'job',
      size: 260,
      header: () => <span className="text-muted">Job</span>,
      cell: ({ row }) => {
        const item = row.original
        return (
          <div className="flex flex-col min-w-0 gap-0.5">
            <a
              href={item.job_url}
              target="_blank"
              rel="noopener noreferrer"
              onClick={(e) => e.stopPropagation()}
              title={item.job_title}
              className="font-semibold text-primary text-sm truncate leading-tight hover:text-violet hover:underline"
            >
              {item.job_title}
            </a>
            <span className="text-[13px] text-secondary truncate">{item.company}</span>
          </div>
        )
      },
    },

    // ── 2. Location ──────────────────────────────────────────────────────────
    {
      id: 'location',
      size: 180,
      enableSorting: false,
      header: () => <span className="text-muted">Location</span>,
      cell: ({ row }) => (
        <JobLocationCell remote={row.original.remote} location={row.original.location} />
      ),
    },

    // ── 3. Salary ──────────────────────────────────────────────────────────
    {
      id: 'salary',
      size: 140,
      enableSorting: false,
      header: () => <span className="text-muted">Salary</span>,
      cell: ({ row }) => {
        const salary = formatSalary(row.original.salary_range)
        return salary ? (
          <span className="font-mono text-[13px] font-semibold text-salary">{salary}</span>
        ) : (
          <span className="text-[13px] text-placeholder">Not listed</span>
        )
      },
    },

    // ── 4. Status ────────────────────────────────────────────────────────────
    {
      id: 'status',
      size: 130,
      enableSorting: false,
      header: () => <span className="text-muted">Status</span>,
      cell: ({ row }) => <ApplyQueueStatusPill status={row.original.status} />,
    },

    // ── 5. Posted (added) ────────────────────────────────────────────────────
    {
      accessorKey: 'added_at',
      id: 'added_at',
      size: 100,
      header: ({ column }) => <DataTableColumnHeader column={column} title="Posted" />,
      cell: ({ getValue }) => {
        const addedAt = getValue() as string
        return (
          <span
            className="text-[12.5px] text-muted"
            title={`Added ${formatExactDateTime(addedAt)}`}
          >
            {formatRelativeTime(addedAt)}
          </span>
        )
      },
    },

    // ── 6. Resume picker ───────────────────────────────────────────────────
    {
      id: 'resume',
      size: 220,
      enableSorting: false,
      header: () => <span className="text-muted">Resume</span>,
      cell: ({ row }) => {
        const item = row.original
        const effective = getEffectiveResume(item.job_id)
        return (
          <ResumePickerCell
            jobId={item.job_id}
            resumeId={effective?.id ?? null}
            resumeLabel={effective?.label ?? null}
            options={resumeOptions}
            onChange={onResumeChange}
            disabled={isResumeUpdatePending?.(item.job_id) ?? false}
          />
        )
      },
    },

    // ── 7. Match ─────────────────────────────────────────────────────────────
    // One-line "N/M matched" summary against the effective resume — only
    // meaningful once a resume is selected (auto-picked or user-chosen).
    {
      id: 'match',
      size: 110,
      enableSorting: false,
      header: () => <span className="text-muted">Match</span>,
      cell: ({ row }) => {
        const summary = getMatchSummary(row.original.job_id)
        return summary ? (
          <span className="text-[12.5px] font-mono text-secondary">{summary}</span>
        ) : (
          <span className="text-[12.5px] text-placeholder">—</span>
        )
      },
    },
  ]
}
```

- [ ] **Step 4: Export `EffectiveResume` from the feature barrel**

`useApplyQueueResumeMatch` (Task I6) lives under `pages/` and needs this type — add it to `frontend/src/features/apply-queue/index.ts` alongside the existing `ApplyQueueResumeOption` export:

```ts
export type { ApplyQueueResumeOption } from './components/ResumePickerCell'
export type { EffectiveResume } from './components/apply-queue-columns'
```

(Insert this next to the file's existing `export type { ApplyQueueResumeOption }` line rather than duplicating it — check the current line first.)

- [ ] **Step 5: Run the test**

Run: `cd frontend && npx vitest run src/features/apply-queue/components/__tests__/apply-queue-columns.test.tsx`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/features/apply-queue/components/apply-queue-columns.tsx frontend/src/features/apply-queue/components/__tests__/apply-queue-columns.test.tsx frontend/src/features/apply-queue/index.ts
git commit -m "feat(apply-queue): add match-summary column, resolve effective resume per row"
```

### Task I5: Thread the new props through `ApplyQueueTable.tsx`

**Files:**

- Modify: `frontend/src/features/apply-queue/components/ApplyQueueTable.tsx`
- Modify: `frontend/src/features/apply-queue/components/__tests__/ApplyQueueTable.test.tsx`

- [ ] **Step 1: Update the props interface and `useMemo` dependency list**

```tsx
import { useMemo } from 'react'
import { DataTable } from '@/components/ui/data-table'
import { Pagination } from '@/components/ui/pagination'
import { cn } from '@/lib/utils/utils'
import type { ApplyQueueItem } from '../api/apply-queue.api'
import { buildApplyQueueColumns, type ApplyQueueColumnActions } from './apply-queue-columns'

interface ApplyQueueTableProps {
  items: ApplyQueueItem[]
  loading: boolean
  isError?: boolean
  onRetry?: () => void
  emptyMessage?: string

  // Resume picker + match wiring — injected by the pages layer (see
  // ResumePickerCell.tsx for why this feature never imports features/resumes
  // directly).
  resumeOptions: ApplyQueueColumnActions['resumeOptions']
  onResumeChange: ApplyQueueColumnActions['onResumeChange']
  isResumeUpdatePending?: ApplyQueueColumnActions['isResumeUpdatePending']
  getEffectiveResume: ApplyQueueColumnActions['getEffectiveResume']
  getMatchSummary: ApplyQueueColumnActions['getMatchSummary']

  // Row-click wiring — opens the job detail panel at the pages layer.
  selectedJobId?: string | null
  onRowClick?: (item: ApplyQueueItem) => void

  // Cursor pagination — prev/next only (not infinite scroll).
  page: number
  itemsOnPage: number
  itemsBeforePage: number
  limit: number
  onLimitChange: (limit: number) => void
  hasNextPage: boolean
  hasPrevPage: boolean
  onNext: () => void
  onPrev: () => void
  onFirst?: () => void
}

function rowLabel(item: ApplyQueueItem): string {
  return `${item.job_title}${item.company ? ` at ${item.company}` : ''}, status ${item.status}`
}

export function ApplyQueueTable({
  items,
  loading,
  isError,
  onRetry,
  emptyMessage = 'No jobs in your apply queue yet',
  resumeOptions,
  onResumeChange,
  isResumeUpdatePending,
  getEffectiveResume,
  getMatchSummary,
  selectedJobId,
  onRowClick,
  page,
  itemsOnPage,
  itemsBeforePage,
  limit,
  onLimitChange,
  hasNextPage,
  hasPrevPage,
  onNext,
  onPrev,
  onFirst,
}: ApplyQueueTableProps) {
  const columns = useMemo(
    () =>
      buildApplyQueueColumns({
        resumeOptions,
        onResumeChange,
        isResumeUpdatePending,
        getEffectiveResume,
        getMatchSummary,
      }),
    [resumeOptions, onResumeChange, isResumeUpdatePending, getEffectiveResume, getMatchSummary]
  )

  return (
    <div className="flex flex-col flex-1 overflow-hidden min-w-0">
      <div className="flex-1 overflow-auto px-6">
        <DataTable
          columns={columns}
          data={items}
          loading={loading}
          isError={isError}
          onRetry={onRetry}
          errorMessage="Couldn't load your apply queue"
          emptyMessage={emptyMessage}
          getRowId={(item) => item.job_id}
          selectedId={selectedJobId ?? undefined}
          onRowClick={onRowClick}
          getRowClassName={(item) => {
            const isActive = selectedJobId === item.job_id
            return cn('group/row', isActive ? 'bg-[var(--accent-dim)]' : 'hover:bg-hover')
          }}
          getRowProps={
            onRowClick
              ? (item) => ({
                  tabIndex: 0,
                  role: 'button',
                  'aria-label': rowLabel(item),
                  'aria-selected': selectedJobId === item.job_id ? true : undefined,
                  onKeyDown: (e: React.KeyboardEvent<HTMLTableRowElement>) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      onRowClick(item)
                    }
                  },
                  className:
                    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet focus-visible:ring-inset',
                })
              : undefined
          }
        />
      </div>

      <div className="border-t border-[var(--border)] shrink-0">
        <Pagination
          mode="cursor"
          page={page}
          itemsOnPage={itemsOnPage}
          itemsBeforePage={itemsBeforePage}
          limit={limit}
          onLimitChange={onLimitChange}
          hasNext={hasNextPage}
          hasPrev={hasPrevPage}
          onNext={onNext}
          onPrev={onPrev}
          onFirst={onFirst}
          loading={loading}
        />
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Update `ApplyQueueTable.test.tsx`**

Run: `grep -n "resumeOptions\|recommended" frontend/src/features/apply-queue/components/__tests__/ApplyQueueTable.test.tsx` and add `getEffectiveResume={() => null}` and `getMatchSummary={() => null}` (or meaningfully-configured versions for tests that exercise them) to every `<ApplyQueueTable ... />` render call in this file, since both props are now required.

- [ ] **Step 3: Run the test**

Run: `cd frontend && npx vitest run src/features/apply-queue/components/__tests__/ApplyQueueTable.test.tsx`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/apply-queue/components/ApplyQueueTable.tsx frontend/src/features/apply-queue/components/__tests__/ApplyQueueTable.test.tsx
git commit -m "feat(apply-queue): thread getEffectiveResume/getMatchSummary through ApplyQueueTable"
```

### Task I6: Write the page-level `useApplyQueueResumeMatch` hook

**Files:**

- Create: `frontend/src/pages/apply-queue/useApplyQueueResumeMatch.ts`
- Test: `frontend/src/pages/apply-queue/__tests__/useApplyQueueResumeMatch.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useApplyQueueResumeMatch } from '../useApplyQueueResumeMatch'
import type { ApplyQueueItem } from '@/features/apply-queue'
import type { Resume } from '@/features/resumes'

function makeItem(overrides: Partial<ApplyQueueItem>): ApplyQueueItem {
  return {
    job_id: 'job-1',
    job_title: 'Engineer',
    company: 'Acme',
    job_url: 'https://x',
    status: 'saved',
    resume_id: null,
    resume_label: null,
    job_keywords: [],
    location: null,
    remote: false,
    salary_range: null,
    added_at: '2026-07-01T00:00:00Z',
    ...overrides,
  }
}

function makeResume(overrides: Partial<Resume>): Resume {
  return {
    id: 'r1',
    user_id: 'u1',
    label: 'My resume',
    original_filename: 'r.pdf',
    content_type: 'application/pdf',
    size_bytes: 1024,
    created_at: '2026-01-01T00:00:00Z',
    extracted_text: '',
    ...overrides,
  }
}

describe('useApplyQueueResumeMatch', () => {
  it('auto-picks the best-matching resume when none is attached', () => {
    const items = [
      makeItem({ job_id: 'job-1', job_keywords: ['Go', 'PostgreSQL'], resume_id: null }),
    ]
    const resumes = [makeResume({ id: 'r1', extracted_text: 'Go and PostgreSQL experience.' })]
    const { result } = renderHook(() => useApplyQueueResumeMatch(items, resumes))
    expect(result.current.getEffectiveResume('job-1')).toEqual({ id: 'r1', label: 'My resume' })
    expect(result.current.getMatchSummary('job-1')).toBe('2/2 matched')
  })

  it('prefers the user-attached resume over the auto-pick', () => {
    const items = [
      makeItem({
        job_id: 'job-1',
        job_keywords: ['Go', 'PostgreSQL'],
        resume_id: 'r2',
        resume_label: 'Manually chosen',
      }),
    ]
    const resumes = [
      makeResume({ id: 'r1', label: 'Best match', extracted_text: 'Go and PostgreSQL.' }),
      makeResume({
        id: 'r2',
        label: 'Manually chosen',
        extracted_text: 'Totally unrelated content.',
      }),
    ]
    const { result } = renderHook(() => useApplyQueueResumeMatch(items, resumes))
    expect(result.current.getEffectiveResume('job-1')).toEqual({
      id: 'r2',
      label: 'Manually chosen',
    })
    expect(result.current.getMatchSummary('job-1')).toBe('0/2 matched')
  })

  it('returns null for both when the job has no keywords', () => {
    const items = [makeItem({ job_id: 'job-1', job_keywords: [], resume_id: null })]
    const resumes = [makeResume({ id: 'r1' })]
    const { result } = renderHook(() => useApplyQueueResumeMatch(items, resumes))
    expect(result.current.getEffectiveResume('job-1')).toBeNull()
    expect(result.current.getMatchSummary('job-1')).toBeNull()
  })

  it('returns null for an unknown jobId', () => {
    const { result } = renderHook(() => useApplyQueueResumeMatch([], []))
    expect(result.current.getEffectiveResume('nope')).toBeNull()
    expect(result.current.getMatchSummary('nope')).toBeNull()
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd frontend && npx vitest run src/pages/apply-queue/__tests__/useApplyQueueResumeMatch.test.ts`
Expected: FAIL — module does not exist.

- [ ] **Step 3: Implement the hook**

```ts
import { useMemo } from 'react'
import { matchResume, pickBestResume, type ResumeForMatch } from '@/lib/utils/resumeJobMatch'
import type { ApplyQueueItem, EffectiveResume } from '@/features/apply-queue'
import type { Resume } from '@/features/resumes'

// useApplyQueueResumeMatch computes, per apply-queue row, which resume is
// "effectively" selected (the user's explicit attachment, falling back to
// the best client-side auto-pick) and a one-line match summary against it.
// Lives under pages/ (not features/) because it composes data from both
// features/apply-queue and features/resumes, which may not import each
// other — see features/CLAUDE.md.
//
// The auto-pick is purely a display computation: it is never written back
// to the backend as a real attachment. If the user attaches a different
// resume (or re-confirms the same one), that's a real write via the existing
// setApplyQueueResume mutation, unrelated to this hook.
export function useApplyQueueResumeMatch(items: ApplyQueueItem[], resumes: Resume[]) {
  const resumesForMatch: ResumeForMatch[] = useMemo(
    () => resumes.map((r) => ({ id: r.id, label: r.label, extractedText: r.extracted_text })),
    [resumes]
  )

  const autoPickByJobId = useMemo(() => {
    const map = new Map<string, EffectiveResume>()
    for (const item of items) {
      const best = pickBestResume(item.job_keywords, resumesForMatch)
      if (best) map.set(item.job_id, best)
    }
    return map
  }, [items, resumesForMatch])

  const itemByJobId = useMemo(() => {
    const map = new Map<string, ApplyQueueItem>()
    for (const item of items) map.set(item.job_id, item)
    return map
  }, [items])

  function getEffectiveResume(jobId: string): EffectiveResume | null {
    const item = itemByJobId.get(jobId)
    if (!item) return null
    if (item.resume_id) return { id: item.resume_id, label: item.resume_label ?? 'Unknown resume' }
    return autoPickByJobId.get(jobId) ?? null
  }

  function getMatchSummary(jobId: string): string | null {
    const item = itemByJobId.get(jobId)
    if (!item) return null
    const effective = getEffectiveResume(jobId)
    if (!effective) return null
    const resume = resumesForMatch.find((r) => r.id === effective.id)
    if (!resume) return null
    const { matched, total } = matchResume(item.job_keywords, resume.extractedText)
    if (total === 0) return null
    return `${matched}/${total} matched`
  }

  return { getEffectiveResume, getMatchSummary }
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd frontend && npx vitest run src/pages/apply-queue/__tests__/useApplyQueueResumeMatch.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/pages/apply-queue/useApplyQueueResumeMatch.ts frontend/src/pages/apply-queue/__tests__/useApplyQueueResumeMatch.test.ts
git commit -m "feat(apply-queue): add page-level resume-match hook (auto-pick + summary)"
```

### Task I7: Wire the hook into `ApplyQueuePage.tsx`

**Files:**

- Modify: `frontend/src/pages/ApplyQueuePage.tsx`

- [ ] **Step 1: Update the page**

```tsx
import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { SegmentedSwitch } from '@/components/ui/segmented-switch'
import { Sheet, SheetContent, SheetTitle, SheetDescription } from '@/components/ui/sheet'
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
import {
  useApplyQueue,
  useSetApplyQueueResume,
  ApplyQueueTable,
  applyQueueQueryKeys,
  APPLY_QUEUE_FILTER,
  type ApplyQueueFilter,
  type ApplyQueueResumeOption,
  type ApplyQueueItem,
} from '@/features/apply-queue'
import { useResumes } from '@/features/resumes'
import { useJob, useJobActions, JobDetailPanel } from '@/features/jobs'
import type { Job, JobStatus } from '@/types'
import { useApplyQueueResumeMatch } from './apply-queue/useApplyQueueResumeMatch'

export function ApplyQueuePage() {
  const q = useApplyQueue()
  const { data: resumes } = useResumes()
  const setResume = useSetApplyQueueResume()
  const queryClient = useQueryClient()
  const jobActions = useJobActions()
  const { getEffectiveResume, getMatchSummary } = useApplyQueueResumeMatch(q.items, resumes ?? [])

  const [selectedJobId, setSelectedJobId] = useState<string | null>(null)
  const selectedJob = useJob(selectedJobId ?? undefined)

  const resumeOptions: ApplyQueueResumeOption[] = (resumes ?? []).map((r) => ({
    id: r.id,
    label: r.label,
  }))

  function handleResumeChange(jobId: string, resumeId: string | null) {
    setResume.mutate({ jobId, resumeId })
  }

  function isResumeUpdatePending(jobId: string): boolean {
    return setResume.isPending && setResume.variables?.jobId === jobId
  }

  function invalidateApplyQueue() {
    void queryClient.invalidateQueries({ queryKey: applyQueueQueryKeys.all() })
  }

  function handlePanelClose() {
    setSelectedJobId(null)
  }

  function handleRowClick(item: ApplyQueueItem) {
    setSelectedJobId(item.job_id)
  }

  function handleStatusChange(job: Job, status: Exclude<JobStatus, 'dismissed' | 'saved'>) {
    jobActions.setStatus.mutate({ id: job.id, status }, { onSuccess: invalidateApplyQueue })
  }

  function handleToggleSave(job: Job) {
    const currentlySaved = job.user_state?.saved_at != null
    jobActions.setSaved.mutate(
      { id: job.id, saved: !currentlySaved },
      { onSuccess: invalidateApplyQueue }
    )
  }

  function handleDismiss(job: Job) {
    jobActions.dismiss.mutate(
      { id: job.id, reason: 'user_dismissed' },
      { onSuccess: invalidateApplyQueue }
    )
    setSelectedJobId(null)
  }

  return (
    <div className="flex-1 flex flex-col min-h-0 overflow-hidden">
      <div className="flex items-center justify-between gap-4 px-6 py-5 border-b border-[var(--border)] shrink-0 flex-wrap">
        <div>
          <h1 className="text-2xl font-semibold text-primary">Apply Queue</h1>
          <p className="text-sm text-muted mt-1">
            Jobs you've saved or applied to — attach a resume to each before you apply.
          </p>
        </div>
        <SegmentedSwitch<ApplyQueueFilter>
          aria-label="Apply queue filter"
          value={q.filter}
          onChange={q.setFilter}
          options={[
            { value: APPLY_QUEUE_FILTER.ALL, label: 'All', accent: 'violet' },
            { value: APPLY_QUEUE_FILTER.SAVED, label: 'Saved', accent: 'violet' },
          ]}
        />
      </div>

      <ErrorBoundary>
        <ApplyQueueTable
          items={q.items}
          loading={q.isLoading}
          isError={q.isError}
          onRetry={q.refetch}
          emptyMessage={
            q.filter === APPLY_QUEUE_FILTER.SAVED
              ? 'No saved jobs yet'
              : 'No jobs in your apply queue yet'
          }
          resumeOptions={resumeOptions}
          onResumeChange={handleResumeChange}
          isResumeUpdatePending={isResumeUpdatePending}
          getEffectiveResume={getEffectiveResume}
          getMatchSummary={getMatchSummary}
          selectedJobId={selectedJobId}
          onRowClick={handleRowClick}
          page={q.page}
          itemsOnPage={q.itemsOnPage}
          itemsBeforePage={q.itemsBeforePage}
          limit={q.limit}
          onLimitChange={q.setLimit}
          hasNextPage={q.hasNextPage}
          hasPrevPage={q.hasPrevPage}
          onNext={q.goToNextPage}
          onPrev={q.goToPrevPage}
          onFirst={q.goToFirstPage}
        />
      </ErrorBoundary>

      <Sheet
        modal={false}
        open={!!selectedJobId}
        onOpenChange={(open) => {
          if (!open) handlePanelClose()
        }}
      >
        <SheetContent
          side="right"
          showOverlay={false}
          showCloseButton={false}
          onPointerDownOutside={(e) => e.preventDefault()}
          onInteractOutside={(e) => e.preventDefault()}
          className="w-[32rem] flex flex-col p-0 border-l border-[var(--border)] shadow-[0_8px_24px_rgba(0,0,0,0.4)]"
        >
          {selectedJobId && (
            <>
              <SheetTitle className="sr-only">
                {selectedJob.data?.title ?? 'Job details'}
              </SheetTitle>
              <SheetDescription className="sr-only">
                {selectedJob.data?.company?.name ?? ''} job details
              </SheetDescription>
              {selectedJob.isLoading ? (
                <div className="flex flex-1 flex-col gap-3 p-6">
                  <div className="h-6 w-2/3 animate-pulse rounded bg-subtle" />
                  <div className="h-4 w-1/3 animate-pulse rounded bg-subtle" />
                  <div className="mt-4 h-32 animate-pulse rounded bg-subtle" />
                </div>
              ) : selectedJob.isError || !selectedJob.data ? (
                <div className="flex flex-1 flex-col items-start gap-3 p-6">
                  <p className="text-sm text-red-400">Couldn't load this job.</p>
                  <button
                    type="button"
                    onClick={handlePanelClose}
                    className="rounded-lg border border-[var(--border-strong)] bg-subtle px-3 py-1.5 text-sm text-secondary"
                  >
                    Close
                  </button>
                </div>
              ) : (
                <JobDetailPanel
                  job={selectedJob.data}
                  onClose={handlePanelClose}
                  onStatusChange={handleStatusChange}
                  onToggleSave={handleToggleSave}
                  onDismiss={handleDismiss}
                />
              )}
            </>
          )}
        </SheetContent>
      </Sheet>
    </div>
  )
}
```

(Only the `useApplyQueueResumeMatch` import/call and the two new `ApplyQueueTable` props are new — everything else is unchanged from the current file.)

- [ ] **Step 2: Confirm the page still builds and lints — full test update happens in Task I8**

Run: `cd frontend && npx tsc -b --noEmit 2>&1 | grep -i "ApplyQueuePage\|apply-queue" | head -30`
Expected: type errors only in test files (Task I8 not yet done) — the page component itself should type-check clean.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/pages/ApplyQueuePage.tsx
git commit -m "feat(apply-queue): wire useApplyQueueResumeMatch into ApplyQueuePage"
```

### Task I8: Update `pages/__tests__/ApplyQueuePage.test.tsx` and `resumes` sample fixture everywhere

**Files:**

- Modify: `frontend/src/pages/__tests__/ApplyQueuePage.test.tsx`

- [ ] **Step 1: Update the fixtures**

```tsx
const sampleItem = {
  job_id: 'job-1',
  job_title: 'Senior Engineer',
  company: 'Acme',
  job_url: 'https://acme.example/jobs/1',
  status: 'saved',
  resume_id: null,
  resume_label: null,
  job_keywords: ['Go', 'PostgreSQL'],
  location: 'Austin, TX',
  remote: false,
  salary_range: null,
  added_at: '2026-06-01T00:00:00Z',
}

const sampleResume = {
  id: 'resume-1',
  user_id: 'u1',
  label: 'My resume',
  original_filename: 'resume.pdf',
  content_type: 'application/pdf' as const,
  size_bytes: 2048,
  created_at: '2026-07-03T00:00:00Z',
  extracted_text: 'Backend engineer with Go and PostgreSQL experience.',
}
```

- [ ] **Step 2: Replace the "Recommended badge" test with an auto-pick/match test**

Replace `it('shows a Recommended badge on the option matching recommended_resume_id', ...)` with:

```tsx
it('auto-picks the best-matching resume and shows the match summary when none is attached', async () => {
  fetchApplyQueueMock.mockResolvedValue({ items: [sampleItem], next_cursor: '' })
  fetchResumesMock.mockResolvedValue([sampleResume])
  render(<ApplyQueuePage />, { wrapper: makeWrapper() })

  await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())

  expect(screen.getByText('2/2 matched')).toBeInTheDocument()
  expect(screen.getByRole('combobox', { name: /select resume/i })).toHaveTextContent('My resume')
})

it('shows a placeholder in the match column when no resume matches', async () => {
  fetchApplyQueueMock.mockResolvedValue({
    items: [{ ...sampleItem, job_keywords: ['Rust'] }],
    next_cursor: '',
  })
  fetchResumesMock.mockResolvedValue([sampleResume])
  render(<ApplyQueuePage />, { wrapper: makeWrapper() })

  await waitFor(() => expect(screen.getByText('Senior Engineer')).toBeInTheDocument())
  expect(screen.getByText('—')).toBeInTheDocument()
})
```

- [ ] **Step 3: Update the "selecting a resume" test if it depended on `recommended_resume_id` state — otherwise leave as-is**

Confirm `it('selecting a resume in the picker calls the mutation and invalidates the list', ...)` still passes unchanged (it uses `sampleItem` with `resume_id: null` and clicks "My resume" from the dropdown — this flow is unaffected by the auto-pick/match changes, since auto-pick only affects the _displayed_ value, not the dropdown's option list or the mutation call).

- [ ] **Step 4: Run the full page test file**

Run: `cd frontend && npx vitest run src/pages/__tests__/ApplyQueuePage.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/pages/__tests__/ApplyQueuePage.test.tsx
git commit -m "test(apply-queue): update ApplyQueuePage tests for job_keywords + auto-pick match summary"
```

---

## Part J — Full frontend verification

### Task J1: Lint, typecheck, full test run

- [ ] **Step 1: Lint**

Run: `cd frontend && npm run lint`
Expected: 0 issues.

- [ ] **Step 2: Typecheck**

Run: `cd frontend && npx tsc -b --noEmit`
Expected: 0 errors. (Use `tsc -b`, not bare `tsc --noEmit` — the latter silently checks zero files in this repo.)

- [ ] **Step 3: Full test suite with coverage**

Run: `cd frontend && npx vitest run --coverage`
Expected: all tests pass; 90%+ statement coverage on every touched file (`resumeJobMatch.ts`, `useApplyQueueResumeMatch.ts`, `cacheConfig.ts`, and every modified component/hook/API file).

- [ ] **Step 4: Fix anything short before proceeding**

---

## Part K — End-to-end manual verification

### Task K1: Verify the full flow against the real local stack

- [ ] **Step 1: Start the dev stack**

Run: `make dev` (per `.claude/skills/run/SKILL.md`) — starts API on `:8080` and Vite on `:5173`.

- [ ] **Step 2: Upload a resume with real extractable text**

In the browser at `http://localhost:5173`, upload a real PDF/DOCX resume via the Resumes page. Confirm: upload succeeds, no "Extract keywords" button/badges appear on the card.

- [ ] **Step 3: Upload a resume that will fail extraction (e.g. a scanned image-only PDF, or a tiny near-empty file)**

Confirm: the upload itself fails with a clear error message (not a silent success with no keywords).

- [ ] **Step 4: Visit the Apply Queue page with at least one job that has `job_tech_tags` populated**

Confirm: the Match column shows a sensible "N/M matched" value once a resume is auto-picked or manually attached; the resume picker shows the auto-picked resume as selected when nothing has been manually attached yet.

- [ ] **Step 5: Manually pick a different resume in the dropdown**

Confirm: the picker now shows the manual choice, the Match column updates to reflect it, and reloading the page preserves the manual choice (persisted via `resume_job_attachments`) rather than reverting to the auto-pick.

- [ ] **Step 6: Confirm no regressions on the Jobs page / job detail panel tech-stack display** (broadened `techtag.Clean` output flows through `job_tech_tags`, not the `jobs.tech_stack` column the Jobs page renders from — so this should show no visible change; verify that assumption holds)

---

## Part L — Spec doc

### Task L1: Write the implementation spec

- [ ] **Step 1: After all parts above are complete and verified, write a brief spec to `docs/superpowers/specs/` with today's date, per `CLAUDE.md`'s "After implementation" rule — file-level references only, no code snippets, no filler.**
