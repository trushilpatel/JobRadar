---
name: tech-debt
description: Use when you want to calculate, score, and plan the reduction of tech debt in a codebase area. Triggers on "calculate tech debt", "score this area", "how bad is the debt in X", "what needs refactoring in Y", "debt report for Z", "baseline this area". Dispatches two parallel specialist agents (engineering principles + dev-ergonomics), synthesizes a ranked debt report with fix-delta estimates, and commits a plan doc to docs/tech-debt/.
---

# Tech Debt Calculator

## Overview

Dispatches two read-only specialist agents in parallel to score a codebase area against engineering principles and developer ergonomics. A synthesizer deduplicates findings, attaches a qualitative fix-delta estimate to each item, ranks by a weighted debt score, and commits a plan doc to `docs/tech-debt/`.

**Required sub-skill:** `superpowers:dispatching-parallel-agents` — both specialists run in parallel.

This skill is **read-only during scoring** — no code edits. Output is one committed Markdown file at `docs/tech-debt/YYYY-MM-DD-<scope>-debt.md`.

---

## When to Use

- Before starting a refactor — get a baseline and prioritize
- After a feature ships — measure what it left behind
- When an area "feels messy" and you want a concrete score
- Periodically on the full repo to track debt trend over time

**Do NOT use for:**

- Fixing anything → use `feature-team` or `production-team-loop`
- Correctness / security bugs → use `quality-team`
- Broad backlog triage → use `next-actions`

---

## Inputs

| Input        | Required | Description                                                                          |
| ------------ | -------- | ------------------------------------------------------------------------------------ |
| `SCOPE`      | Yes      | Path (`backend/service/company`), area name (`"all service slices"`), or `full-repo` |
| `DIMENSIONS` | No       | Subset to score. Default: all. E.g. `"SOLID, coupling, doc-drift"`                   |
| `FOCUS`      | No       | Concern to weight heavier. E.g. `"magic strings"`, `"extensibility"`                 |
| `BASELINE`   | No       | Path to a prior debt doc — synthesizer computes delta to track trend over time       |

**Examples:**

```
/tech-debt backend/service/scan
/tech-debt backend/service/scan FOCUS="coupling"
/tech-debt full-repo
/tech-debt backend/service/company BASELINE=docs/tech-debt/2026-05-01-company-debt.md
```

---

## Step 1 — Load Context

Before dispatching, read:

- Root `CLAUDE.md` and every area-level `CLAUDE.md` inside `SCOPE`
- `.claude/rules/engineering-principles.md` — the principles Agent 1 scores against
- `.claude/rules/backend-solid.md` — Go-specific SOLID patterns
- `.claude/rules/dev-ergonomics.md` — the 10 ergonomics dimensions Agent 2 scores against
- `BASELINE` doc if provided

---

## Step 2 — Dispatch Two Specialists (Parallel)

Use `superpowers:dispatching-parallel-agents`. Both run simultaneously. Feed each agent the loaded context from Step 1.

---

### Agent 1 — Engineering Principles Scorer

**Scores against:** `.claude/rules/engineering-principles.md` + `.claude/rules/backend-solid.md`

**Dimensions to score (assign grade A–F to each):**

| Dimension                        | What to look for                                                                                                                                                                    |
| -------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **SOLID**                        | SRP violations (file doing N things), OCP violations (switch chains instead of registry), ISP violations (fat interfaces), DIP violations (concrete type imports across boundaries) |
| **Loose coupling**               | Concrete types leaking across package boundaries; files outside SCOPE that must change when SCOPE changes (shotgun surgery); shared mutable state                                   |
| **No magic values**              | Bare status/reason/stage strings not defined as constants; inline numeric literals for timeouts/limits/retries; hardcoded path strings in frontend outside `api/*.ts`               |
| **Extensibility**                | "List of things" expressed as scattered switch/if-else instead of a registry or config map; adding a new variant requires editing > 2 files outside the feature dir                 |
| **Test-first**                   | Missing tests for identified edge cases; flaky/skipped tests without removal condition; test files that test the happy path only                                                    |
| **Correctness over convenience** | TODOs, hacks, or "fix later" comments that indicate a known wrong approach; code that works around a structural problem instead of fixing it                                        |

**Evidence-gathering commands (run for SCOPE):**

```bash
# SRP: files over 300 lines in SCOPE
find <SCOPE> -name "*.go" -o -name "*.ts" -o -name "*.tsx" | xargs wc -l 2>/dev/null | sort -rn | head -20

# Magic strings: bare status/reason strings
grep -rn '"new"\|"saved"\|"applied"\|"dismissed"\|"error"\|"retry"\|"fetch"\|"filtered"' <SCOPE> --include="*.go" | grep -v "_test.go\|constants\|const "

# Concrete type imports across boundaries
grep -rn "service\.\|store\." <SCOPE> --include="*.go" | grep -v "interface\|_test.go"

# Switch chains that should be registries
grep -rn "switch.*type\|switch.*status\|switch.*reason\|switch.*adapter" <SCOPE> --include="*.go"

# TODOs / hacks
grep -rn "TODO\|FIXME\|HACK\|XXX\|workaround\|// temp\|// fix later" <SCOPE>

# Skipped/flaky tests
grep -rn "t\.Skip\|t\.SkipNow\|// flaky\|xfail" <SCOPE> --include="*_test.go"

# Missing test files (packages with no _test.go)
find <SCOPE> -name "*.go" ! -name "*_test.go" | sed 's|/[^/]*$||' | sort -u | while read d; do ls "$d"/*_test.go 2>/dev/null || echo "NO TESTS: $d"; done
```

**Output format per finding:**

```
[EP][Grade: X→Y] <title> — <dimension>
Evidence: `file:line` or grep result
Debt: what makes this costly to maintain or extend
Fix: the specific change in ≤2 sentences
Fix delta: <dimension> moves from <current grade> to <projected grade>
Effort: S (<2h) / M (half-day) / L (multi-day)
```

**Scorecard output:**

```
## Engineering Principles Scorecard
| Dimension        | Grade | Top finding |
| SOLID            | X     | ...         |
| Loose coupling   | X     | ...         |
| No magic values  | X     | ...         |
| Extensibility    | X     | ...         |
| Test-first       | X     | ...         |
| Correctness      | X     | ...         |
```

---

### Agent 2 — Dev-Ergonomics Scorer

**Scores against:** `.claude/rules/dev-ergonomics.md` (all 10 dimensions)

Read the rule file verbatim. Score each dimension A–F using the friction rubric:
`score = (frequency×0.40) + (reach×0.35) + (fix-cost⁻¹×0.25)`

Apply hard P0 overrides:

- A `CLAUDE.md` statement false about current code → Hard P0
- A quality gate that can be silently bypassed → Hard P0
- A file on a hot change path past ~800 lines → Hard P0
- An intermittently flaky test → Hard P0

**Evidence-gathering commands:**

```bash
# Discoverability: missing area CLAUDE.md
find <SCOPE> -maxdepth 2 -type d | while read d; do [ -f "$d/CLAUDE.md" ] || echo "NO CLAUDE.md: $d"; done

# Context locality: large files
find <SCOPE> -name "*.go" -o -name "*.ts" -o -name "*.tsx" | xargs wc -l 2>/dev/null | sort -rn | head -20

# One blessed path: duplicate helpers
grep -rn "func.*Format\|func.*Parse\|func.*Convert\|func.*Build" <SCOPE> --include="*.go" | grep -v "_test.go"

# Doc-truth: verify CLAUDE.md claims match code
# (read each area CLAUDE.md and grep for each named file/table/flag)

# Guardrails: bypassable gates
grep -rn "t\.Skip\|--no-verify\|// nolint\|coverage-exclude\|/* istanbul ignore" <SCOPE>

# Scaffolding: count files touched to add one new unit
# (count files modified in last 3 PRs that added a new service/adapter)
git log --oneline --diff-filter=A -10 -- <SCOPE>
```

**Output format:** same structure as Agent 1 but using dev-ergonomics dimension names and friction scoring.

**Scorecard output:**

```
## Dev-Ergonomics Scorecard
| Dimension             | Grade | Friction score | Top finding |
| Discoverability       | X     | N.N            | ...         |
| Feedback-loop speed   | X     | N.N            | ...         |
| One blessed path      | X     | N.N            | ...         |
| Guardrails            | X     | N.N            | ...         |
| Context locality      | X     | N.N            | ...         |
| Debuggability         | X     | N.N            | ...         |
| Onboarding            | X     | N.N            | ...         |
| Doc-truth             | X     | N.N            | ...         |
| Safe change           | X     | N.N            | ...         |
| Scaffolding           | X     | N.N            | ...         |
```

---

## Step 3 — Synthesize

Receive both agent reports. Produce the committed debt doc at `docs/tech-debt/YYYY-MM-DD-<scope>-debt.md`.

**Deduplication rule:** if the same file/pattern appears in both reports, merge into one finding assigned to the highest-severity dimension. Do not list the same item twice.

**Debt Index:** compute a single 0–100 score where 0 = no debt, 100 = maximum debt.

```
debt_index = (count_of_F_grades × 20 + count_of_D_grades × 12 + count_of_C_grades × 6 + count_of_B_grades × 2) / total_dimensions
capped at 100
```

**Debt doc structure:**

```markdown
# Tech Debt Report: <SCOPE>

**Date:** YYYY-MM-DD  
**Debt Index:** NN/100  
**Trend:** ↑NN / ↓NN / — (if BASELINE provided, else omit)

## Scorecard

| Layer | Dimension | Grade | Top Finding |
| Engineering | SOLID | X | ... |
| Engineering | Loose coupling | X | ... |
| Engineering | No magic values | X | ... |
| Engineering | Extensibility | X | ... |
| Engineering | Test-first | X | ... |
| Engineering | Correctness | X | ... |
| Dev-Ergonomics | Discoverability | X | ... |
| Dev-Ergonomics | One blessed path | X | ... |
| Dev-Ergonomics | Context locality | X | ... |
| Dev-Ergonomics | Doc-truth | X | ... |
| Dev-Ergonomics | Safe change | X | ... |
| Dev-Ergonomics | Scaffolding | X | ... |
(remaining ergonomics dimensions)

## Findings (ranked by debt weight)

### P0 — Fix immediately

### P1 — Fix this sprint

### P2 — Backlog

### P3 — Nice-to-have

Each finding:
**[PN] Title** — `file:line`

- **Dimension:** X
- **Grade:** X → Y after fix
- **Debt:** what makes this costly
- **Fix:** specific change
- **Effort:** S / M / L

## Proposed Fix Order

Ordered sequence considering: P0 first, then quick wins (S effort), then structural refactors (L effort). Each item as a ready-to-paste prompt:
`/feature-team <specific task description> in <file/package>`

## Trend Analysis

(Only if BASELINE provided)

- Dimensions that improved since last run
- Dimensions that regressed
- Net debt index change

## Assets — What's Working

2–4 things this area does well (so a fix pass doesn't break them)
```

**After writing the doc:** commit it.

```bash
git add docs/tech-debt/
git commit -m "docs(tech-debt): <scope> debt report — index NN/100"
```

---

## Common Mistakes

| Mistake                                 | Fix                                                                 |
| --------------------------------------- | ------------------------------------------------------------------- |
| Scoring everything as F                 | Apply the rubric strictly — reserve F for genuinely broken patterns |
| Listing raw grep output as findings     | Every finding must be human-readable with a concrete fix            |
| Synthesizer not deduplicating           | Merge findings that name the same file/pattern                      |
| Forgetting the Debt Index               | Always compute it — it's the trend-tracking anchor                  |
| Proposed fix prompts that are too vague | Every prompt must be specific enough to invoke immediately          |
| Skipping the "Assets" section           | Mandatory — stops a fix pass from regressing good patterns          |

---

## Self-Improvement

After this skill completes:

1. Invoke `skill-self-improvement` to check whether any finding warrants updating this file or `CLAUDE.md`.
2. **Engineering principles evolution:** if any finding reveals a systematic violation that is not yet captured in `.claude/rules/engineering-principles.md` — a pattern the current rules don't prohibit but should — flag it as a candidate addition and route it through `skill-self-improvement`. Principle changes require human review before landing; never write them autonomously.
