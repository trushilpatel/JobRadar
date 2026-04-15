import { cn } from '@/lib/utils'

export type SegmentedAccent = 'violet' | 'emerald' | 'red' | 'blue' | 'amber'

export interface SegmentedOption<T extends string> {
  value: T
  label: string
  count?: number
  accent?: SegmentedAccent
}

interface SegmentedSwitchProps<T extends string> {
  options: SegmentedOption<T>[]
  value: T
  onChange: (v: T) => void
  'aria-label'?: string
}

const ACCENT_ON: Record<SegmentedAccent, string> = {
  violet:  'bg-violet-500/20 text-violet-300',
  emerald: 'bg-emerald-500/[0.18] text-emerald-300',
  red:     'bg-red-500/[0.18] text-red-300',
  blue:    'bg-blue-500/20 text-blue-300',
  amber:   'bg-amber-500/20 text-amber-300',
}

export function SegmentedSwitch<T extends string>({
  options, value, onChange, ...rest
}: SegmentedSwitchProps<T>) {
  return (
    <div
      role="radiogroup"
      aria-label={rest['aria-label']}
      className="inline-flex bg-white/[0.03] border border-white/[0.08] rounded-lg p-0.5"
    >
      {options.map(opt => {
        const on = opt.value === value
        const accent = opt.accent ?? 'violet'
        return (
          <button
            key={opt.value}
            type="button"
            role="radio"
            aria-checked={on}
            onClick={() => onChange(opt.value)}
            className={cn(
              'inline-flex items-center gap-1.5 text-[11px] px-2.5 py-1 rounded-md transition-colors whitespace-nowrap',
              on ? ACCENT_ON[accent] : 'text-slate-400 hover:text-slate-200',
              on && 'shadow-[0_1px_2px_rgba(0,0,0,0.25)]',
            )}
          >
            {opt.label}
            {opt.count !== undefined && (
              <span className="text-[10px] bg-black/25 px-1.5 py-px rounded-full">
                {opt.count}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}
