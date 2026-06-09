# Workflow Rules

## Edge-case discovery before implementation

Before writing any implementation code for a non-trivial feature or fix:

1. Dispatch a **separate read-only agent** whose only job is to enumerate edge cases, failure modes, and boundary conditions for the feature
2. The implementation agent must not start until the edge-case list is reviewed and confirmed
3. Every identified edge case must have a corresponding test — not a comment, not a TODO, a test
4. "Simple" features are not exempt — if the feature truly is simple, the edge-case agent confirms that quickly and the cost is negligible

This is the same 3-phase flow described in `CLAUDE.md` (edge-case discovery → test plan → implementation), applied universally — not just for per-service test backfill.

## Never write design specs

Do not create design specification files, architecture documents, or spec markdown files unless explicitly asked. Implement directly.

## No unsolicited git actions or questions

- Never ask about committing, pushing, branching, or PRs unless the user brings it up
- Never suggest "should I commit this?" or "want me to create a PR?"
- Only perform git operations when explicitly instructed in that message
