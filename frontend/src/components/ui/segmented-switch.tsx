import type { ReactNode } from 'react'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { cn } from '@/lib/utils/utils'

export type SegmentedAccent = 'violet' | 'emerald' | 'red' | 'blue' | 'amber'

export interface SegmentedOption<T extends string> {
  value: T
  label: ReactNode
  count?: number
  accent?: SegmentedAccent
}

interface SegmentedSwitchProps<T extends string> {
  options: SegmentedOption<T>[]
  value: string
  onChange: (v: T) => void
  disabled?: boolean
  'aria-label'?: string
}

const ACCENT_ON: Record<SegmentedAccent, string> = {
  violet: 'data-[state=on]:bg-violet-500/20 data-[state=on]:text-violet-300',
  emerald: 'data-[state=on]:bg-emerald-500/[0.18] data-[state=on]:text-emerald-300',
  red: 'data-[state=on]:bg-red-500/[0.18] data-[state=on]:text-red-300',
  blue: 'data-[state=on]:bg-blue-500/20 data-[state=on]:text-blue-300',
  amber: 'data-[state=on]:bg-amber-500/20 data-[state=on]:text-amber-300',
}

export function SegmentedSwitch<T extends string>({
  options,
  value,
  onChange,
  disabled = false,
  ...rest
}: SegmentedSwitchProps<T>) {
  return (
    <ToggleGroup
      type="single"
      value={value}
      onValueChange={(v) => {
        // ToggleGroup fires '' when the active item is clicked again; ignore to keep exactly one selected
        if (v) onChange(v as T)
      }}
      aria-label={rest['aria-label']}
      className="inline-flex bg-subtle border border-[var(--border)] rounded-lg p-0.5 gap-0"
    >
      {options.map((opt) => {
        const accent = opt.accent ?? 'violet'
        return (
          <ToggleGroupItem
            key={opt.value}
            value={opt.value}
            disabled={disabled}
            className={cn(
              'inline-flex items-center gap-1.5 text-[11px] px-2.5 py-1 rounded-md transition-colors whitespace-nowrap h-auto min-w-0',
              'text-muted hover:text-primary hover:bg-transparent',
              'data-[state=on]:shadow-[0_1px_2px_rgba(0,0,0,0.25)]',
              ACCENT_ON[accent]
            )}
          >
            {opt.label}
            {opt.count !== undefined && (
              <span className="text-[10px] bg-black/25 px-1.5 py-px rounded-full">{opt.count}</span>
            )}
          </ToggleGroupItem>
        )
      })}
    </ToggleGroup>
  )
}
