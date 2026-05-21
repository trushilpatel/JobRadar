---
name: git-push-to-pr
description: Use when there are local changes (staged, unstaged, or committed on main) that need to be moved onto a new feature branch, rebased on top of origin/main, and pushed to origin ready for a PR.
---

# Git Push to PR

## Overview

Package local changes into a clean feature branch rebased on top of the latest `origin/main`, then push to origin. Follows this project's commit conventions and hook requirements.

**Core principle:** Never commit to main. Always rebase, never merge. Hooks are the floor — never skip them.

## Step 1 — Assess state

```bash
git status
git branch           # which branch am I on?
git log --oneline -3 # any commits already on this branch?
```

| State                                  | Action                                           |
| -------------------------------------- | ------------------------------------------------ |
| On `main` with unstaged/staged changes | Create branch now (changes follow automatically) |
| On `main` with committed changes       | Create branch from current HEAD                  |
| Already on a feature branch            | Skip to Step 3 — just stage and commit           |
| Nothing to commit                      | Stop — nothing to push                           |

## Step 2 — Create the branch

Only do this if on `main`. Infer the branch name from the nature of the changes. Format: `<type>/<short-description>` using lowercase kebab-case.

```bash
git checkout -b <type>/<short-description>
# e.g. chore/consolidate-env-files  feat/add-heatmap  fix/crawler-timeout
```

**Type prefixes:** `feat` · `fix` · `chore` · `docs` · `refactor` · `test` · `perf`

The branch name is permanent — make it descriptive, not generic.

## Step 3 — Stage changes

Prefer named files over `git add -A`. Keeps unrelated noise out of the commit and avoids accidentally staging secrets or build artefacts.

```bash
git add path/to/file1 path/to/file2
git rm --cached path/to/deleted-file   # for tracked deletions
git status                             # verify what's staged before committing
```

## Step 4 — Commit

```bash
git commit -m "type(scope): short imperative description

Optional longer body explaining WHY, not what.
Wrap at 72 chars."
```

**Conventional commit rules:**

- First line: `type(scope): description` — imperative mood, no period, ≤72 chars
- Scope is optional but useful (e.g. `env`, `crawler`, `api`, `ui`)
- Body only if the why is non-obvious
- **Never** add `Co-Authored-By: Claude` or any AI marker

**Pre-commit hook runs automatically:** gofmt · golangci-lint · Go unit tests (race) · frontend lint · TypeScript check. If it fails, fix the underlying issue and recommit — never use `--no-verify`.

## Step 5 — Fetch latest main

```bash
git fetch origin
```

## Step 6 — Rebase onto origin/main

```bash
git rebase origin/main
```

### If conflicts arise

```bash
# 1. Open each conflicted file and resolve manually
git status                    # see which files are conflicted

# 2. After resolving each file:
git add <resolved-file>

# 3. Continue:
git rebase --continue

# 4. Repeat for each conflict round until rebase completes
```

**Never** use `git rebase --skip` unless the commit is genuinely empty after resolution (confirm with `git diff HEAD`). If the rebase becomes badly tangled, abort and start over:

```bash
git rebase --abort
```

## Step 7 — Push

```bash
git push -u origin <branch-name>
```

If the branch already exists on origin and you need to update it (only for your own feature branch, never main):

```bash
git push --force-with-lease origin <branch-name>
```

Never `--force` (no lease) — it silently overwrites remote commits.

## Quick reference

```bash
# Full happy path (on main, unstaged changes)
git checkout -b feat/my-feature
git add src/foo.go src/bar.go
git commit -m "feat(api): add rate-limit header to response"
git fetch origin
git rebase origin/main
git push -u origin feat/my-feature
```

## Common mistakes

| Mistake                                      | Fix                                                   |
| -------------------------------------------- | ----------------------------------------------------- |
| `git add .` or `git add -A` without checking | Always run `git status` first; stage by name          |
| Committing to `main`                         | The pre-commit hook blocks it — create a branch first |
| Using `--no-verify` when hook fails          | Fix the lint/test failure; never skip                 |
| Adding AI attribution to commit message      | Omit it entirely — human author only                  |
| `git push --force` (no lease)                | Always `--force-with-lease` if force is needed        |
| Skipping `git fetch` before rebase           | You'll rebase onto stale main — always fetch first    |

## Red flags — stop and reassess

- You're about to run `git commit` on the `main` branch
- You're considering `--no-verify`
- You're about to `git push --force` (not `--force-with-lease`)
- The rebase produced more conflicts than expected — `git rebase --abort` and verify you fetched first
