import type { CommandRegistry, CommandScope, KeySpec } from './types'
import { isMac } from './platform'

type BucketName = 'global' | 'list' | 'useKey'

/**
 * The result of compiling a registry into the structures the keyboard engine
 * needs.
 *
 * - The three `*Keys` strings are comma-joined `react-hotkeys-hook` bindings,
 *   one per listener bucket. For chord commands (`g>g`) only the PREFIX key
 *   (`g`) appears here — react-hotkeys-hook v5 has no native key-sequence
 *   support, so the engine completes chords itself.
 * - `lookup` maps a normalized single-combo fired key to a command id (used for
 *   non-chord commands).
 * - `chords` maps a normalized prefix step to `{ secondStep → id }` so the
 *   engine can finish a two-step sequence after the prefix fires.
 */
export interface Dispatch {
  globalKeys: string
  listKeys: string
  useKeyKeys: string
  /** normalizedKey → command id (single-combo, non-chord commands). */
  lookup: Map<string, string>
  /** prefixStep → (secondStep → id). */
  chords: Map<string, Map<string, string>>
  /** Set of normalized prefix steps that begin a chord (for engine routing). */
  chordPrefixes: Set<string>
}

/** Canonical modifier ordering so `meta+shift+r` === `shift+meta+r`. */
const MODIFIER_ORDER = ['meta', 'ctrl', 'alt', 'shift']

const MODIFIER_ALIASES: Record<string, string> = {
  cmd: 'meta',
  command: 'meta',
  control: 'ctrl',
  option: 'alt',
}

/**
 * Canonical aliases for non-modifier keys so the registry and the engine's
 * event-derived key agree. `/` → `slash`; `?` is shift+slash and is rewritten
 * in `normalizeKey`.
 */
const KEY_ALIASES: Record<string, string> = {
  '/': 'slash',
  return: 'enter',
  esc: 'escape',
  arrowleft: 'left',
  arrowright: 'right',
  arrowup: 'up',
  arrowdown: 'down',
}

const isDev = (): boolean => {
  try {
    return Boolean(import.meta.env?.DEV)
  } catch {
    return false
  }
}

/** Expands `mod` to the platform modifier and canonicalises aliases. */
function expandToken(token: string): string {
  const t = token.trim().toLowerCase()
  if (t === 'mod') return isMac() ? 'meta' : 'ctrl'
  return MODIFIER_ALIASES[t] ?? t
}

/**
 * Normalizes one chord step (`shift+meta+r`) into a canonical, modifier-sorted
 * form. Throws on a malformed step (empty token, dangling `+`).
 */
function normalizeChordStep(step: string, spec: KeySpec): string {
  if (step.trim() === '') {
    throw new Error(`buildDispatch: malformed key "${spec}" — empty chord step`)
  }
  // `?` is the shifted slash — canonicalise so it collides with `shift+slash`.
  if (step.trim() === '?') return 'shift+slash'
  const tokens = step.split('+').map(expandToken)
  if (tokens.some((tok) => tok === '')) {
    throw new Error(`buildDispatch: malformed key "${spec}" — empty token`)
  }
  const modifiers: string[] = []
  const plain: string[] = []
  for (const tok of tokens) {
    if (MODIFIER_ORDER.includes(tok)) modifiers.push(tok)
    else plain.push(KEY_ALIASES[tok] ?? tok)
  }
  modifiers.sort((a, b) => MODIFIER_ORDER.indexOf(a) - MODIFIER_ORDER.indexOf(b))
  return [...modifiers, ...plain].join('+')
}

/**
 * Normalizes a full KeySpec, including chords (`g>g`). Exported so the engine
 * resolves a fired hotkey through the same canonical form the maps are keyed
 * by.
 */
export function normalizeKey(spec: KeySpec): string {
  if (spec.trim() === '') {
    throw new Error(`buildDispatch: malformed key "${spec}" — empty`)
  }
  const steps = spec.split('>')
  if (steps.some((s) => s.trim() === '')) {
    throw new Error(`buildDispatch: malformed key "${spec}" — empty chord step`)
  }
  return steps.map((step) => normalizeChordStep(step, spec)).join('>')
}

/**
 * Compiles a registry into dispatch structures. In dev builds it throws on
 * duplicate keys within a bucket, malformed keys, and a bare key that is the
 * prefix of another command's chord. `overrides` replaces a command's keys —
 * the free remap seam (otherwise unused here).
 */
export function buildDispatch(
  registry: CommandRegistry,
  overrides: Record<string, KeySpec[]> = {}
): Dispatch {
  const lookup = new Map<string, string>()
  const chords = new Map<string, Map<string, string>>()
  const allChordPrefixes = new Set<string>()

  // Per-bucket collision tracking: registered key (prefix for chords) → id.
  const seen: Record<BucketName, Map<string, string>> = {
    global: new Map(),
    list: new Map(),
    useKey: new Map(),
  }
  // Registered react-hotkeys-hook key strings per bucket (dedup-preserving).
  const bucketKeys: Record<BucketName, Set<string>> = {
    global: new Set(),
    list: new Set(),
    useKey: new Set(),
  }
  // Per-bucket bare keys + chord prefixes for the bare-vs-chord guard.
  const barePrefixes: Record<BucketName, Set<string>> = {
    global: new Set(),
    list: new Set(),
    useKey: new Set(),
  }
  const chordPrefixesByBucket: Record<BucketName, Set<string>> = {
    global: new Set(),
    list: new Set(),
    useKey: new Set(),
  }
  const dev = isDev()

  for (const [id, def] of Object.entries(registry)) {
    const keys = overrides[id] ?? def.keys
    const scope: CommandScope = def.scope ?? 'global'
    const bucketName: BucketName = def.useKey ? 'useKey' : scope

    for (const spec of keys) {
      const normalized = normalizeKey(spec)
      const steps = normalized.split('>')
      const isChord = steps.length > 1
      const prefix = steps[0] ?? normalized

      if (dev) {
        // Collision is per (bucket, normalized full key).
        const prior = seen[bucketName].get(normalized)
        if (prior && prior !== id) {
          throw new Error(
            `buildDispatch: duplicate key "${normalized}" claimed by both "${prior}" and "${id}" in scope "${bucketName}"`
          )
        }
        if (isChord) chordPrefixesByBucket[bucketName].add(prefix)
        else barePrefixes[bucketName].add(normalized)
      }
      seen[bucketName].set(normalized, id)

      if (isChord) {
        const second = steps[1] ?? ''
        const byPrefix = chords.get(prefix) ?? new Map<string, string>()
        byPrefix.set(second, id)
        chords.set(prefix, byPrefix)
        allChordPrefixes.add(prefix)
        // react-hotkeys-hook has no native key sequences, so BOTH the prefix
        // and the completion key must be registered for their `keydown` to fire
        // the listener — the engine then resolves the two-step sequence itself.
        bucketKeys[bucketName].add(prefix)
        bucketKeys[bucketName].add(second)
      } else {
        lookup.set(normalized, id)
        bucketKeys[bucketName].add(normalized)
      }
    }
  }

  if (dev) {
    const bucketNames: BucketName[] = ['global', 'list', 'useKey']
    for (const bucketName of bucketNames) {
      for (const bare of barePrefixes[bucketName]) {
        if (chordPrefixesByBucket[bucketName].has(bare)) {
          throw new Error(
            `buildDispatch: bare key "${bare}" collides with a chord prefix in scope "${bucketName}" — a bare prefix key would swallow the chord`
          )
        }
      }
    }
  }

  const join = (set: Set<string>): string => [...set].join(', ')

  return {
    globalKeys: join(bucketKeys.global),
    listKeys: join(bucketKeys.list),
    useKeyKeys: join(bucketKeys.useKey),
    lookup,
    chords,
    chordPrefixes: allChordPrefixes,
  }
}
