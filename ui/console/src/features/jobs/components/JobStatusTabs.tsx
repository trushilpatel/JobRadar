import { cn } from '@/lib/utils'
import type { TabKey } from '@/features/jobs/hooks/useJobs'

const TABS: { key: TabKey; label: string }[] = [
  { key: 'new', label: 'New' },
  { key: 'saved', label: 'Saved' },
  { key: 'applied', label: 'Applied' },
  { key: 'interviewing', label: 'Interviewing' },
  { key: 'offer', label: 'Offer' },
  { key: 'rejected', label: 'Rejected' },
]

interface JobStatusTabsProps {
  tab: TabKey
  onChange: (tab: TabKey) => void
  counts: Partial<Record<TabKey, number>>
  leftSlot?: React.ReactNode
  rightSlot?: React.ReactNode
}

export function JobStatusTabs({ tab, onChange, counts, leftSlot, rightSlot }: JobStatusTabsProps) {
  return (
    <div className="flex items-end gap-0.5 px-6 border-b border-white/[0.07]">
      {leftSlot && <div className="flex items-center pb-2 mr-4 shrink-0">{leftSlot}</div>}
      {TABS.map((t) => (
        <button
          key={t.key}
          onClick={() => onChange(t.key)}
          className={cn(
            'px-4 py-2 text-sm font-medium transition-colors rounded-t-lg border-b-2 -mb-px',
            tab === t.key
              ? 'text-violet-300 border-violet-500 bg-violet-500/5'
              : 'text-slate-400 border-transparent hover:text-white hover:bg-white/[0.03]'
          )}
        >
          {t.label}
          {counts[t.key] != null && (
            <span
              className={cn(
                'ml-1.5 text-xs px-1.5 py-0.5 rounded-full',
                tab === t.key
                  ? 'bg-violet-500/20 text-violet-300'
                  : 'bg-white/[0.08] text-slate-500'
              )}
            >
              {counts[t.key]}
            </span>
          )}
        </button>
      ))}
      {rightSlot && <div className="flex items-center pb-2 ml-auto shrink-0">{rightSlot}</div>}
    </div>
  )
}
