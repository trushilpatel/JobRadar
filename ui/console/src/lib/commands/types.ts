/**
 * A `react-hotkeys-hook` key string. Supports `mod+` (expanded to `meta` on
 * Mac, `ctrl` elsewhere by the engine), chords (`g>g`) and aliases.
 */
export type KeySpec = string

/** Grouping used by the palette and help sheet. */
export type CommandCategory = 'navigation' | 'tabs' | 'status' | 'search'

/**
 * `'global'` fires anywhere except form inputs. `'list'` is additionally
 * suppressed inside popovers/dialogs so the popover's own key nav wins.
 */
export type CommandScope = 'global' | 'list'

/**
 * The static, declarative half of a command — on-screen text plus key
 * bindings. `keys: []` makes the command palette/help-only (never registered
 * with the keyboard engine).
 */
export interface CommandDef {
  /** On-screen text (replaces the old `copy` field). */
  label: string
  /** Key bindings; aliases (e.g. `['j','down']`) fan out to the same handler. */
  keys: KeySpec[]
  /** Optional palette/help grouping. */
  category?: CommandCategory
  /** Optional scope; defaults to `'global'`. */
  scope?: CommandScope
  /**
   * When true, the binding is registered with `react-hotkeys-hook`'s
   * `useKey: true` so it matches by produced character across layouts
   * (e.g. `/`, `?`).
   */
  useKey?: boolean
}

/** A registry maps a command id to its static definition. */
export type CommandRegistry = Record<string, CommandDef>

/**
 * The runtime half — one handler per command id. Because the value type is
 * keyed by the id union, omitting a handler is a compile error.
 */
export type CommandHandlers<Id extends string> = Record<Id, (e: KeyboardEvent) => void>
