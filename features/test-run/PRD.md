# PRD — [Feature Name]

> Human-authored. Never modified by agents.
> Fill in every section. Vague PRDs fail the linter and block the loop.

---

## Overview

<!-- What are we building and why? One paragraph. -->

**Feature slug**: `feature-name-kebab-case`
**Author**: [Your name]
**Date**: YYYY-MM-DD
**Priority**: P0 / P1 / P2

---

## Problem Statement

<!-- What user or business problem does this solve? Be specific. -->

---

## Users

<!-- Who uses this feature? Define user types if more than one. -->

| User Type | Description |
| --------- | ----------- |
| ...       | ...         |

---

## Acceptance Criteria

<!-- Each criterion must be: testable, specific, scoped to one thing. -->
<!-- Use "must" and "must not". No "should", "could", "fast", "easy". -->

- [ ] **AC-1**: [User type] must be able to [action] and the system must [observable outcome].
- [ ] **AC-2**: When [condition], the system must [behaviour].
- [ ] **AC-3**: The API must respond with [specific payload] in < [N]ms at p95 under [load].
- [ ] **AC-4**: If [error condition], the system must return [error response] and must not [harmful side effect].

---

## Out of Scope

<!-- Explicitly list what this feature does NOT include. Prevents scope creep. -->

- ...

---

## Performance Budgets (override defaults if needed)

<!-- Leave blank to use system defaults from CLAUDE.md -->

| Metric           | Budget                   |
| ---------------- | ------------------------ |
| p95 API response | ms (default: 300ms)      |
| Error rate       | % (default: <1%)         |
| Bundle delta     | KB (default: ≤20KB gzip) |

---

## Security Constraints

<!-- Any specific auth/authz requirements, data sensitivity, or compliance constraints -->

---

## Dependencies

<!-- Other features, services, or APIs this depends on -->

---

## Open Questions

<!-- Things you're unsure about — the linter will flag these too if they're in ACs -->

- [ ] ...

---

## Definition of Done

A feature is done when:

1. All ACs have green tests
2. Mutation score ≥ 70%
3. k6 perf budgets met
4. Red team P0 findings = 0
5. UX P0/P1 resolved
6. PR is open and human has approved
