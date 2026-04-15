import { cn } from '@/lib/utils'
import type { TabKey } from '@/features/jobs/hooks/useJobs'

const TABS: { key: TabKey; label: string }[] = [
  { key: 'new',     label: 'New' },
  { key: 'saved',   label: 'Saved' },
  { key: 'applied', label: 'Applied' },
]

interface JobStatusTabsProps {
  tab: TabKey
  total: number
  onChange: (tab: TabKey) => void
}

export function JobStatusTabs({ tab, total, onChange }: JobStatusTabsProps) {
  return (
    <div className="flex items-center gap-0.5 px-6 pt-3 border-b border-white/[0.07]">
      {TABS.map(t => (
        <button
          key={t.key}
          onClick={() => onChange(t.key)}
          className={cn(
            'px-4 py-2 text-sm font-medium transition-colors rounded-t-lg border-b-2 -mb-px',
            tab === t.key
              ? 'text-violet-300 border-violet-500 bg-violet-500/5'
              : 'text-slate-400 border-transparent hover:text-white hover:bg-white/[0.03]',
          )}
        >
          {t.label}
          {tab === t.key && total > 0 && (
            <span className="ml-1.5 text-xs px-1.5 py-0.5 rounded-full bg-violet-500/20 text-violet-300">
              {total}
            </span>
          )}
        </button>
      ))}
    </div>
  )
}
