---
name: next-actions
description: Use when deciding what to work on next, the backlog feels unclear, or you want a prioritized view of bugs, tech debt, feature gaps, quality gaps, and stale work before starting anything new. Triggers on "what should I work on?", "what's next?", "show me the backlog", "what's broken", "what's missing".
---

# Next Actions

## Overview

Dispatches five specialist agents in parallel to scan the codebase and git state across ten quality dimensions, then a synthesizer consolidates findings into a prioritised action plan — four buckets (Bugs, Tech Debt, Feature Gaps, Quality/Observability) with ready-to-paste prompts for `feature-team` or `quality-team` as follow-ups.

This skill is **read-only**. It never writes code. It surfaces what exists; the human decides what to start.

**Required sub-skill:** `superpowers:dispatching-parallel-agents` — all five specialists run in parallel.

---

## When to Use

- "What should I work on next?"
- "What's broken / missing / incomplete?"
- "Show me the backlog"
- After a major feature ships — to find what it left behind
- Before planning a sprint — to get a codebase-grounded view of open work
- After a long break — to re-orient on project state

**Do NOT use for:**

- Implementing anything (use `feature-team`)
- Deep auditing of one specific area (use `quality-team`)
- Debugging a specific failing test (use `superpowers:systematic-debugging`)

---

## Priority Scoring Rubric

Every finding is scored on three axes — the synthesizer uses this to rank across categories:

| Axis                    | Weight | Score 3                                                 | Score 2                                       | Score 1                                  |
| ----------------------- | ------ | ------------------------------------------------------- | --------------------------------------------- | ---------------------------------------- |
| **Blast radius**        | 40%    | Data loss / auth bypass / pipeline stall / broken build | Wrong output, degraded perf, user-visible bug | Cosmetic, internal tooling, dev-only     |
| **Detection lag**       | 35%    | Silent (no log, no error, no alert)                     | Delayed (noticed in hours/days)               | Immediate (error surfaced to user or CI) |
| **Fix cost** (inverted) | 25%    | S — under 2h                                            | M — half-day                                  | L — multi-day                            |

**Final score = (blast × 0.40) + (detection_lag × 0.35) + (fix_cost_inverted × 0.25)**

| Score   | Priority | Action                                |
| ------- | -------- | ------------------------------------- |
| ≥ 2.5   | **P0**   | Fix before anything else              |
| 1.8–2.4 | **P1**   | Next piece of work                    |
| 1.2–1.7 | **P2**   | Backlog — address this sprint or next |
| < 1.2   | **P3**   | Nice-to-have                          |

**Hard overrides (always P0 regardless of score):**

- Broken build or compilation error anywhere in `backend/apps/`
- Auth bypass or missing `RequireRole` on an admin route
- Silent data drop in `job_pipeline_ledger` or `company_scan_queue`
- Error swallowed without any log entry in a worker loop

---

## Step 1 — Read Project Rules

Before dispatching, read:

- `CLAUDE.md` (root) — pipeline topology, DB schema, key rules
- Any subdirectory `CLAUDE.md` files in areas the scan will touch
- `Designs/` directory — any open design docs signal planned but unbuilt work

This context is fed into each specialist's prompt so findings are project-anchored, not generic.

---

## Step 2 — Dispatch Five Specialists (Parallel)

Use `superpowers:dispatching-parallel-agents`. All five run simultaneously.

---

### Specialist 1 — Bug Scanner

**Reads:** `backend/apps/`, `backend/service/`, `backend/common/`, `frontend/src/`

**Detection signals — run all of these:**

```bash
# Known breakage markers
grep -rn "TODO.*broken\|FIXME\|HACK\|BUG\|panic(" backend/ frontend/src/

# Broken builds — try to build every app binary
cd backend && go build ./apps/... 2>&1

# Swallowed errors (blank-identifier discards)
grep -rn "_ = " backend/service/ backend/apps/

# Worker loops calling os.Exit or log.Fatal (violates error isolation rule)
grep -rn "os.Exit\|log.Fatal" backend/apps/

# Kafka/NATS consumer missing commit after processing
grep -rn "FetchMessage\|Fetch(" backend/apps/ | grep -v "_test.go"
```

**Output format per finding:**

```
[BUG][P{N}] {file:line} — {short title}
Root cause: {what is wrong}
Blast radius: {what breaks, who is affected}
Fix: {concrete change in ≤2 sentences}
Test to add: {assertion that catches this regression}
```

---

### Specialist 2 — Tech Debt Scanner

**Reads:** `backend/service/`, `backend/apps/`, `frontend/src/`

**Detection signals:**

```bash
# Decorator chain violation — raw GORM outside repo layer
grep -rn "\.DB\b\|gorm\.DB" backend/service/ | grep -v "RepositoryService\|_test.go\|testutil"

# Sentinel error violation
grep -rn "fmt\.Errorf.*Err\|errors\.New.*not found\|errors\.New.*not authorized" backend/service/

# Raw JSON encoding in handlers
grep -rn "json\.NewEncoder\|json\.Marshal" backend/apps/

# Stdlib log instead of platform/logger
grep -rn "\"log\"\|log\.Print\|log\.Fatal\|fmt\.Println" backend/service/ backend/apps/

# Raw fetch in frontend
grep -rn "^import.*fetch\|= fetch(" frontend/src/features/ frontend/src/api/

# useEffect + useState for server data (should be TanStack Query)
grep -rn "useEffect\|useState" frontend/src/features/ | grep -v "_test\|// "

# Files over 400 lines — single-concern violation
find backend/service -name "*.go" | xargs wc -l | sort -rn | head -20

# Raw string log field keys instead of constants
grep -rn "\"job_id\"\|\"company_id\"\|\"user_id\"\|\"error\"" backend/service/ backend/apps/
```

**Output format per finding:**

```
[DEBT][HIGH|MED|LOW] {file} — {rule violated}
Rule: {which CLAUDE.md rule}
Impact: {what goes wrong if left}
Fix: {specific change}
Effort: S / M / L
```

---

### Specialist 3 — Feature Gap Analyst

**Reads:** `Designs/`, `CLAUDE.md` pipeline section, `backend/apps/`, `backend/service/`, `frontend/src/`, latest migration files.

**Detection signals:**

```bash
# Design docs with open items
grep -rn "TODO\|not started\|planned\|Phase [0-9].*not" Designs/

# Stubs or placeholders in handlers
grep -rn "StatusNotImplemented\|not implemented\|Coming soon\|placeholder" backend/apps/ frontend/src/

# Orphaned DB columns — migrations add a column but no service reads it
ls backend/db/migrations/ | tail -10   # read recent migrations then grep service for each new column

# Missing ATS adapters or adapters disabled
grep -rn "disabled\|intentionally\|re-enable" backend/service/ backend/apps/

# Frontend routes with no real content
grep -rn "route\|Route\|path=" frontend/src/App.tsx
```

Also read the `Designs/` directory fully. For each design doc, check:

- What phases are complete vs. not started?
- Are there open architecture questions that block progress?
- Does the CLAUDE.md pipeline diagram match the actual binaries in `backend/apps/`?

**Output format per finding:**

```
[INCOMPLETE][P{N}] {feature name}
What exists: {what has been built}
What's missing: {specific gap}
Completion path: {ordered task list}
Definition of done: {how to verify end-to-end}
Skill to invoke: feature-team | quality-team
Prompt: `/feature-team {ready-to-paste scope}`
```

---

### Specialist 4 — Quality & Observability Analyst

**Reads:** `backend/service/`, `backend/apps/`, `.github/workflows/`, `scripts/check-coverage.sh`, `frontend/src/`, `frontend/src/features/`

**Detection signals:**

**CI gates:**

```bash
# What gates exist in CI
cat .github/workflows/*.yml | grep -E "run:|npm run|go test|check-coverage"
# What's missing: npm run test, frontend coverage, NATS integration path
```

**Backend coverage:**

```bash
# Packages with no test files at all
for dir in backend/service/*/; do
  find "$dir" -name "*_test.go" | head -1 | grep -q . || echo "NO TESTS: $dir"
done

# Run coverage check
cd backend && go test -short ./... -coverprofile=/tmp/cov.out 2>/dev/null
scripts/check-coverage.sh /tmp/cov.out 80
```

**Observability:**

```bash
# Missing OTel spans — public service methods without tracing.Start
grep -rn "^func.*Service\b" backend/service/ | grep -v "_test\|interface\|Mock"
# Then check if each implementation has tracing.Start

# Errors logged without structured fields
grep -rn "log\.Error.*fmt\.Sprintf\|log\.Warn.*fmt\.Sprintf" backend/service/ backend/apps/

# Raw string field keys
grep -rn "\"job_id\"\|\"company\"\|\"error\"" backend/service/ backend/apps/ | grep "log\."

# Swallowed errors in worker loops (no log, no return)
grep -rn "if err != nil {" backend/apps/ | grep -v "log\.\|return\|panic"
```

**Frontend tests:**

```bash
# Feature directories with no test files
for dir in frontend/src/features/*/; do
  find "$dir" -name "*.test.tsx" -o -name "*.test.ts" | head -1 | grep -q . || echo "NO TESTS: $dir"
done

# Pages with no tests
find frontend/src/pages -name "*.tsx" | while read f; do
  base=$(basename "$f" .tsx)
  find frontend/src -name "${base}*.test.tsx" | grep -q . || echo "NO TESTS: $f"
done
```

**UX completeness:**

```bash
# Data-fetching components missing loading state
grep -rn "useQuery\|useSuspenseQuery" frontend/src/features/ | grep -v "_test"
# Check each for skeleton / isLoading handling nearby
```

**Output format per finding:**

```
[QUALITY][HIGH|MED|LOW] {area} — {what's missing}
Risk: {what production failure this hides}
Fix: {specific addition}
Effort: S / M / L
Skill: quality-team
Prompt: `/quality-team {scope}`
```

---

### Specialist 5 — Git Analyst

**Reads:** git log, remote branches, stash, recent commit messages

**Detection signals:**

```bash
# All remote branches with age
git for-each-ref --sort=-committerdate refs/remotes/origin \
  --format='%(committerdate:relative) %(refname:short)' | head -30

# Recent revert commits — indicate unresolved issues
git log --oneline --since="60 days ago" | grep -i "revert\|rollback\|undo"

# Branches untouched for >14 days (stale candidates)
git for-each-ref --sort=committerdate refs/remotes/origin \
  --format='%(committerdate:relative) %(refname:short)' | grep -E "weeks|months" | grep -v "main"

# Stash entries that may contain abandoned work
git stash list

# Docs drift — CLAUDE.md mentions files that no longer exist
grep -oE "backend/[^ )'\`]+" CLAUDE.md | sort -u | while read f; do
  [ -e "$f" ] || echo "MISSING: $f (referenced in CLAUDE.md)"
done
```

**Output — no priority scoring, just a list:**

```
Stale branch: {branch name} — last commit {age} — inferred work: {from branch name}
  Action: delete | resume with /feature-team | merge

Revert found: {commit hash} "{message}" — {date}
  Indicates: {what area may still be unstable}

Stash entry: stash@{N} — {description} — age: {when}
  Action: apply and finish | drop

Docs drift: {CLAUDE.md section} references {thing} that no longer exists
```

---

## Step 3 — Synthesize

After all five specialists complete, dispatch one **Synthesizer** with all five reports as input:

> Synthesize these five specialist reports into the final next-actions output.
>
> **Rules:**
>
> - Score every finding using the priority rubric (blast 40% + detection_lag 35% + fix_cost_inverted 25%)
> - Apply hard P0 overrides: broken builds, auth bypasses, silent data drops in ledger/scan_queue
> - Deduplicate: if the same file or component appears in two reports, merge into one item (higher priority wins)
> - Group into four buckets: Bugs/Broken Behavior, Tech Debt, Feature Gaps, Quality/Observability
> - Within each bucket, rank P0 → P1 → P2 → P3
> - Every item must include Where, What, Why, Effort, Skill, and Prompt fields
> - The Prompt field must be a ready-to-paste invocation string for `feature-team` or `quality-team`
> - Git Analyst findings go in a separate "Stale Git Work" section — no priority scoring
> - Pick the single highest-ROI item across all buckets for the "Suggested Starting Point"
>
> Do NOT auto-dispatch anything. Present findings and let the human choose.

---

## Step 4 — Present to User

**Final output structure:**

```
## Situation Snapshot
- What's in the best shape: {one bullet}
- Highest-risk gap: {one bullet}
- Oldest unfinished work: {one bullet}

## Bugs / Broken Behavior
### P0
**[BUG] P0 — {title}**
Where: {file:line}
What: {one sentence}
Why: {blast radius and detection lag}
Effort: S/M/L
Skill: feature-team | manual
Prompt: `/feature-team {scope}`

### P1 / P2 / P3 ...

## Tech Debt
(same per-item format)

## Feature Gaps
(same per-item format)

## Quality / Observability
(same per-item format)

## Stale Git Work
- {branch}: {age} — {inferred work} — Suggested action: {delete | resume | merge}
- ...

## Suggested Starting Point
**{item title}** — score {X.X}
Prompt to start now: `/feature-team {full invocation}` or `/quality-team {full invocation}`
```

---

## Common Mistakes

| Mistake                                                    | Fix                                                                                                                |
| ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Dispatching without reading CLAUDE.md first                | Project rules define what's a violation vs. an intentional choice — read first or findings will be false positives |
| Synthesizer auto-dispatching the top item                  | Never auto-dispatch — present and let the human choose                                                             |
| Scoring everything as P0                                   | Apply the rubric strictly; reserve P0 for the hard overrides and score ≥ 2.5                                       |
| Merging findings from the same file into different buckets | Deduplicate — one file = one item, assigned to the most relevant bucket                                            |
| Git Analyst applying priority scoring to branches          | Git findings are informational only — no P0/P1/P2 for branch age                                                   |
| Prompt field left as generic                               | Every Prompt must be specific enough to invoke immediately — name the exact feature, scope, and files              |
| Reporting raw grep output as findings                      | Every finding must be a human-readable task with Fix and Prompt — not a raw grep line                              |

---

## Self-Improvement

After this skill completes, invoke `skill-self-improvement` to check whether any finding warrants updating this file or `CLAUDE.md`.
