import { cn } from '@/lib/utils/utils'

export interface SegmentedTabItem {
  key: string
  label: string
  count?: number
  /**
   * Optional override for this segment's interactive element — used by the
   * Tracking tab to wrap a Popover trigger around the segment instead of a
   * plain button (see JobStatusTabs.tsx). The override owns click handling
   * entirely; `onChange` is NOT called automatically for a rendered item.
   */
  render?: (props: { active: boolean; onSelect: () => void }) => React.ReactNode
}

export interface SegmentedTabsProps {
  items: SegmentedTabItem[]
  value: string
  onChange: (key: string) => void
  className?: string
}

/**
 * Generic pill-segmented control — mirrors the approved mockup's `.tabbar`/
 * `.tab` treatment: a recessed track holding borderless pill segments, the
 * active segment rendered as a raised panel-colored pill (not a color fill).
 * Pure presentational + a per-item render escape hatch — no business logic.
 */
export function SegmentedTabs({ items, value, onChange, className }: SegmentedTabsProps) {
  return (
    <div
      role="tablist"
      className={cn(
        'inline-flex flex-wrap items-center gap-1 rounded-full border border-[var(--border-strong)] bg-subtle p-1',
        'shadow-[inset_0_1px_2px_rgba(0,0,0,0.12)]',
        className
      )}
    >
      {items.map((item) => {
        const active = item.key === value
        if (item.render) {
          return (
            <span key={item.key} role="presentation">
              {item.render({ active, onSelect: () => onChange(item.key) })}
            </span>
          )
        }
        return (
          <button
            key={item.key}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => onChange(item.key)}
            className={cn(
              'inline-flex h-7 items-center gap-1.5 whitespace-nowrap rounded-full px-3.5 text-[13px] font-medium transition-all',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet',
              active
                ? 'bg-panel text-primary shadow-sm ring-1 ring-inset ring-[color-mix(in_srgb,var(--violet)_16%,transparent)]'
                : 'text-muted hover:text-primary hover:bg-[var(--bg-hover)]'
            )}
          >
            <span>{item.label}</span>
            {item.count !== undefined && (
              <span
                className={cn(
                  'font-mono text-[11px] tabular-nums',
                  // `violet` is a flat token, not a numbered shade scale —
                  // `text-violet-600`-style classes silently fail to generate.
                  active ? 'text-[var(--violet-bright)]' : 'text-muted'
                )}
              >
                {item.count > 999 ? '999+' : item.count}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}
