# UI Copy & Keyboard Shortcut Registry — Design

**Date**: 2026-05-29
**Status**: Approved — PRD written at `AutoCodeEngine/features/ui-copy-shortcut-registry/PRD.md`

---

## What we're building

A centralized token registry for all frontend UI strings, with optional keyboard shortcut annotations. Shortcut badge visibility is user-configurable via a backend preferences API. Strongly typed end-to-end.

---

## Architecture

### Token shape

One interface for all tokens. `shortcut` is optional — tokens without it are plain copy tokens:

```
Token {
  label:        string   // display text
  ariaLabel?:   string   // accessibility override
  shortcut?:    string   // display-only ('s', 'meta+k', 'g>n')
  description?: string   // subtitle in ⌘K palette
}
```

No handler is stored in the token. Handlers remain in keyboard hooks. The shortcut string in the token is for display and documentation only — the keyboard hook reads it to avoid duplication.

### File structure

```
config/tokens/
  index.ts            ← merges all feature tokens; exports TOKENS and getActionTokens()
  global.tokens.ts    ← nav labels, page titles, app-wide strings

features/jobs/jobs.tokens.ts
features/companies/companies.tokens.ts
features/settings/settings.tokens.ts
features/auth/auth.tokens.ts
features/dashboard/dashboard.tokens.ts
features/insights/insights.tokens.ts
features/resumes/resumes.tokens.ts
```

Initial PR covers `jobs.tokens.ts` and `global.tokens.ts`. Other features follow.

### Hooks

`useAction(key)` — for interactive tokens. Reads the token registry and the user config. Returns `{ label, ariaLabel, shortcut, kbd }` where `kbd` is a rendered `<Kbd>` element when `show_keyboard_shortcuts` is true, or `null` when false.

`useCopy(key)` — for static strings. Returns the `label` string. No pref check, no kbd.

Token key type is derived from the registry (`keyof typeof TOKENS`) — unknown keys are compile errors.

### User preferences API

New `user_preferences` table: `user_id UUID PK → users(id) ON DELETE CASCADE`, `data JSONB NOT NULL DEFAULT '{}'`, `updated_at TIMESTAMPTZ`.

`GET /api/me/config` — returns the user's `UserConfig`, defaulting to `show_keyboard_shortcuts: false`.
`PATCH /api/me/config` — partial merge via Postgres `||` operator. Returns updated config.

Both behind `RequireIdentity`. `user_id` always from JWT context.

`useUserConfig()` — TanStack Query, `staleTime: 30 min`. PATCH mutation invalidates `['user-config']` on success.

### Typing contract

TypeScript `UserConfig` interface in `types/index.ts` and Go `UserConfig` struct in `service/user/` share identical snake_case JSON field names. Adding a new preference = new field in both structs, no migration.

### Keyboard hook integration

`useJobKeyboard.ts` reads shortcut strings from `features/jobs/jobs.tokens.ts` instead of hardcoding them. One source of truth for shortcut strings — display and binding agree by construction.

### Command palette

`getActionTokens()` exported from `config/tokens/index.ts` returns all tokens with a `shortcut` field. The ⌘K palette uses this list as its action inventory.

---

## Key decisions

| Decision                   | Choice                              | Rationale                                                                                   |
| -------------------------- | ----------------------------------- | ------------------------------------------------------------------------------------------- |
| Preference storage         | Backend API + TanStack Query        | Persists across devices; consistent with app's server-state pattern; single source of truth |
| DB schema                  | JSONB `data` column                 | New prefs = new struct field only, no migration                                             |
| Hook vs component wrapper  | `useAction` hook                    | Works for any element type (buttons, nav, tooltips) — not just `<Button>`                   |
| Shortcut registration      | Keyboard hooks remain authoritative | Avoids duplicate hotkey registrations; `useAction` is display-only                          |
| Token scope for initial PR | Jobs feature + global nav           | Highest density of shortcuts and most visible strings; other features follow                |

---

## Out of scope

- i18n, localization
- Runtime copy editing / CMS
- Custom shortcut rebinding
- Full-app token migration in one PR
