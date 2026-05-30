import { useEffect, useMemo, useRef } from 'react'
import { useHotkeys } from 'react-hotkeys-hook'
import type { CommandHandlers, CommandRegistry } from './types'
import { buildDispatch, normalizeKey, type Dispatch } from './buildDispatch'

/**
 * Minimal shape of the `HotkeysEvent` react-hotkeys-hook passes as the second
 * callback argument (its own type is declared but not exported). `keys` is the
 * list of produced keys; the boolean flags reflect the matched modifiers.
 */
interface HotkeysEventLike {
  keys?: readonly string[]
  meta?: boolean
  ctrl?: boolean
  alt?: boolean
  shift?: boolean
}

/**
 * Options applied to every binding. `react-hotkeys-hook` ignores form inputs
 * by default; we keep that and opt into `preventDefault` so keys like `/`
 * don't trigger browser quick-find.
 */
const BASE_OPTS = {
  preventDefault: true,
  enableOnFormTags: false,
  enableOnContentEditable: false,
} as const

/** Window (ms) within which a second key completes a chord (e.g. `g g`). */
const CHORD_WINDOW_MS = 1000

const MODIFIER_TOKENS = new Set(['meta', 'ctrl', 'control', 'alt', 'option', 'shift'])

/**
 * True when the active element is inside a Radix-style popover (Select
 * listbox, Menu, Combobox, Dialog) or a contenteditable region. List-scope
 * commands (page arrows) are suppressed there so the popover's own keyboard
 * navigation wins. Moved verbatim from the old useJobKeyboard.
 */
function isInPopover(): boolean {
  const el = document.activeElement
  if (!el || el === document.body) return false
  const sel =
    '[role="listbox"], [role="menu"], [role="combobox"], [role="dialog"], [contenteditable], [data-radix-popper-content-wrapper]'
  return !!(el as Element).closest?.(sel)
}

/** Canonicalises a single produced key token to our spec vocabulary. */
function canonToken(raw: string): string {
  const k = raw.trim().toLowerCase()
  const map: Record<string, string> = {
    ' ': 'space',
    space: 'space',
    enter: 'enter',
    return: 'enter',
    escape: 'escape',
    esc: 'escape',
    arrowleft: 'left',
    arrowright: 'right',
    arrowup: 'up',
    arrowdown: 'down',
    '/': 'slash',
    '?': 'slash',
  }
  return map[k] ?? k
}

/**
 * Reconstructs the normalized single-combo key string for the just-fired key.
 *
 * The authoritative base key is the native event's `e.key` (the key that
 * actually triggered this keydown). `react-hotkeys-hook`'s `hk.keys` can carry
 * stale "still-held" keys (it only clears them on keyup/blur), so we use it
 * only as a fallback when the native key is unavailable. Modifier state comes
 * from the native event (reliable), with the HotkeysEvent as a fallback.
 */
function normalizedFromEvent(e: KeyboardEvent, hk: HotkeysEventLike): string | undefined {
  let base = e.key ? canonToken(e.key) : ''
  if (!base || MODIFIER_TOKENS.has(base)) {
    const fromHk = (hk.keys ?? [])
      .map(canonToken)
      .filter((k: string) => k && !MODIFIER_TOKENS.has(k))
    base = fromHk[fromHk.length - 1] ?? ''
  }
  if (!base || MODIFIER_TOKENS.has(base)) return undefined

  const meta = e.metaKey || hk.meta || false
  const ctrl = e.ctrlKey || hk.ctrl || false
  const alt = e.altKey || hk.alt || false
  // `?` is the shifted slash — make sure shift is reflected so it normalizes to
  // `shift+slash` (matching the registry binding for help).
  const shift = e.shiftKey || hk.shift || e.key === '?' || false

  const parts: string[] = []
  if (meta) parts.push('meta')
  if (ctrl) parts.push('ctrl')
  if (alt) parts.push('alt')
  if (shift) parts.push('shift')
  parts.push(base)
  return normalizeKey(parts.join('+'))
}

/**
 * Generic keyboard engine. Compiles the registry into per-bucket bindings and
 * registers a FIXED three `useHotkeys` calls (global, list, useKey). Handlers
 * are read through a ref so callback identity changes never re-register keys.
 *
 * Chords (`g>g`) are completed by the engine: react-hotkeys-hook has no native
 * key-sequence support, so both the prefix and completion keys are registered;
 * when a prefix fires we record it and resolve the second key on the next event.
 */
export function useCommandKeyboard<Id extends string>(
  registry: CommandRegistry,
  handlers: CommandHandlers<Id>,
  opts?: { enabled?: boolean }
): void {
  const enabled = opts?.enabled ?? true
  const dispatch = useMemo(() => buildDispatch(registry), [registry])

  // Latest-ref pattern: keep handlers + dispatch current without re-registering
  // keys. Refs are updated in an effect (never during render) so the
  // refs-during-render lint stays clean.
  const handlersRef = useRef(handlers)
  const dispatchRef = useRef<Dispatch>(dispatch)
  useEffect(() => {
    handlersRef.current = handlers
  }, [handlers])
  useEffect(() => {
    dispatchRef.current = dispatch
  }, [dispatch])

  // Pending chord prefix + the time it fired.
  const pending = useRef<{ prefix: string; at: number } | null>(null)

  const invoke = (id: string, e: KeyboardEvent) => {
    const fn = (handlersRef.current as Record<string, (e: KeyboardEvent) => void>)[id]
    fn?.(e)
  }

  const run = (e: KeyboardEvent, hk: HotkeysEventLike) => {
    const d = dispatchRef.current
    const normalized = normalizedFromEvent(e, hk)
    if (!normalized) return

    // Step 2 of a chord?
    const p = pending.current
    if (p && performance.now() - p.at <= CHORD_WINDOW_MS) {
      const second = d.chords.get(p.prefix)?.get(normalized)
      pending.current = null
      if (second) {
        invoke(second, e)
        return
      }
      // fall through — the key may itself be a standalone command / new prefix
    }

    // A chord prefix? Record and wait for the completion key.
    if (d.chordPrefixes.has(normalized)) {
      pending.current = { prefix: normalized, at: performance.now() }
      return
    }

    // Plain single-combo command.
    const id = d.lookup.get(normalized)
    if (id) invoke(id, e)
  }

  // Empty bucket → useHotkeys('') is a safe no-op. Always three fixed calls.
  useHotkeys(dispatch.globalKeys || '', (e, hk) => run(e, hk), { ...BASE_OPTS, enabled }, [enabled])
  useHotkeys(
    dispatch.listKeys || '',
    (e, hk) => run(e, hk),
    { ...BASE_OPTS, enabled: () => enabled && !isInPopover() },
    [enabled]
  )
  useHotkeys(
    dispatch.useKeyKeys || '',
    (e, hk) => run(e, hk),
    { ...BASE_OPTS, enabled, useKey: true },
    [enabled]
  )
}
