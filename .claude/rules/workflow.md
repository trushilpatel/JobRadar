# Workflow Rules

## Documentation is mandatory, not optional

Every code change that touches a documented area must update that area in the same response. See `.claude/rules/documentation.md` for the full update matrix. Shipping code without updating its docs is incomplete work.

## Do not create new design specs unprompted

Do not create new design specification or architecture documents unless explicitly asked. Implement directly. **Exception:** the post-implementation spec in `docs/superpowers/specs/` is always required after a non-trivial feature or fix — that is not optional.

## Surface architectural notes proactively

After completing any task, scan for systemic issues (missing abstractions, repeated patterns without a home, test gaps, interface bloat). Surface these as short **"Architectural note:"** bullets at the bottom of the response. One sentence each. Do not implement them — flag them for the team to decide.

## No unsolicited git actions or questions

- Never ask about committing, pushing, branching, or PRs unless the user brings it up
- Never suggest "should I commit this?" or "want me to create a PR?"
- Only perform git operations when explicitly instructed in that message

## Production-grade by default

Every change — including small fixes — must meet the same bar as a feature delivery:

- Follows existing code structure and patterns exactly; do not invent new conventions
- Passes all quality gates (backend lint, Go unit tests, frontend lint, TypeScript check) before declaring done
- Updates all affected documentation in the same work unit
- Is safe for a multi-developer team: no hidden assumptions, no global state, no convention breaks
