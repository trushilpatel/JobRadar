import type { KeySpec } from '@/lib/commands/types'
import { formatKeys } from '@/lib/commands/formatKeys'
import { useUserConfig } from '@/lib/hooks/useUserConfig'
import { Kbd, KbdGroup } from './kbd'

export interface ActionView {
  label: string
  /** Primary key spec (first alias) or null when the command has no binding. */
  shortcut: KeySpec | null
  /** `aria-keyshortcuts` value derived from the primary binding, or undefined. */
  ariaKeyshortcuts: string | undefined
  /** Whether the kbd hint should render (user pref + a binding exists). */
  showKbd: boolean
  /** Rendered kbd hint (primary key only), or null. */
  kbd: React.ReactNode | null
}

/**
 * Derives an action button's display + accessibility metadata from a label
 * and its key bindings. Stays in Layer 2 (no feature import) by accepting the
 * already-resolved `label` and `keys` from the caller (e.g. a jobs feature
 * component reading `JOB_COMMANDS`). The kbd hint honours the user's
 * "show keyboard shortcuts" preference; `aria-keyshortcuts` is always set when
 * a binding exists.
 */
export function useAction(label: string, keys: KeySpec[] = []): ActionView {
  const { data: config } = useUserConfig()
  const primary: KeySpec | null = keys[0] ?? null
  const showKbd = !!primary && (config?.show_keyboard_shortcuts ?? false)
  let kbd: React.ReactNode | null = null
  if (showKbd && primary) {
    const groups = formatKeys(primary)
    kbd = (
      <>
        {groups.map((group, gi) => (
          <KbdGroup key={gi}>
            {group.map((glyph, ki) => (
              <Kbd key={`${gi}-${ki}`}>{glyph}</Kbd>
            ))}
          </KbdGroup>
        ))}
      </>
    )
  }
  return {
    label,
    shortcut: primary,
    ariaKeyshortcuts: primary ?? undefined,
    showKbd,
    kbd,
  }
}
