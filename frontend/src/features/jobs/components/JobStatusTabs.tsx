import * as Select from '@radix-ui/react-select'
import { Check, ChevronDown } from 'lucide-react'
import { cn } from '@/lib/utils/utils'
import { useIsMobile } from '@/lib/hooks/use-media-query'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { TabKey } from '../hooks/useJobs'
import { JOB_STATUS, TAB_KEYS } from '@/config/constants/jobStatus'
import { COPY } from '@/config/copy'

const TAB_LABELS: Record<(typeof TAB_KEYS)[number], string> = {
  [JOB_STATUS.NEW]: COPY['jobs.tab.new'],
  [JOB_STATUS.SAVED]: COPY['jobs.tab.saved'],
  [JOB_STATUS.APPLIED]: COPY['jobs.tab.applied'],
  [JOB_STATUS.INTERVIEWING]: COPY['jobs.tab.interviewing'],
  [JOB_STATUS.OFFER]: COPY['jobs.tab.offer'],
  [JOB_STATUS.REJECTED]: COPY['jobs.tab.rejected'],
}

const TABS: { key: TabKey; label: string }[] = TAB_KEYS.map((key) => ({
  key,
  label: TAB_LABELS[key],
}))

interface JobStatusTabsProps {
  tab: TabKey
  onChange: (tab: TabKey) => void
  counts: Partial<Record<TabKey, number>>
  leftSlot?: React.ReactNode
  rightSlot?: React.ReactNode
}

function CountPill({ count, active }: { count: number; active: boolean }) {
  return (
    <span
      data-testid="job-tab-count"
      className={cn(
        'ml-1.5 text-xs px-1.5 py-0.5 rounded-full',
        active ? 'bg-violet-500/20 text-violet-300' : 'bg-subtle text-muted'
      )}
    >
      {count > 999 ? '999+' : count}
    </span>
  )
}

function DesktopTabs({
  tab,
  onChange,
  counts,
}: Omit<JobStatusTabsProps, 'leftSlot' | 'rightSlot'>) {
  return (
    <Tabs
      value={tab}
      onValueChange={(v) => onChange(v as TabKey)}
      className="flex items-end"
      data-testid="job-status-tabs-desktop"
    >
      <TabsList className="flex h-auto items-end gap-0.5 bg-transparent p-0 text-muted">
        {TABS.map((t) => {
          const active = tab === t.key
          const count = counts[t.key]
          return (
            <TabsTrigger
              key={t.key}
              value={t.key}
              className={cn(
                'rounded-none rounded-t-lg border-b-2 -mb-px px-4 py-2 text-sm font-medium transition-colors',
                'shadow-none ring-offset-transparent focus-visible:ring-violet-500',
                'data-[state=inactive]:border-transparent data-[state=inactive]:text-muted data-[state=inactive]:hover:text-primary data-[state=inactive]:hover:bg-hover',
                'data-[state=active]:text-violet-300 data-[state=active]:border-violet-500 data-[state=active]:bg-violet-500/5 data-[state=active]:shadow-none'
              )}
            >
              {t.label}
              {count != null && <CountPill count={count} active={active} />}
            </TabsTrigger>
          )
        })}
      </TabsList>
    </Tabs>
  )
}

function MobileTabSelect({
  tab,
  onChange,
  counts,
}: Omit<JobStatusTabsProps, 'leftSlot' | 'rightSlot'>) {
  const activeLabel = TAB_LABELS[tab]
  const activeCount = counts[tab]

  return (
    <Select.Root value={tab} onValueChange={(v) => onChange(v as TabKey)}>
      <Select.Trigger
        data-testid="job-status-tabs-mobile-trigger"
        aria-label="Select tab"
        className={cn(
          'flex items-center gap-2 px-3 py-1.5 rounded-lg border outline-none cursor-pointer transition-colors',
          'text-sm font-medium text-violet-300 bg-violet-500/10 border-violet-500/20',
          'hover:brightness-110 focus-visible:ring-2 focus-visible:ring-violet-500'
        )}
      >
        <Select.Value>
          <span className="flex items-center">
            {activeLabel}
            {activeCount != null && <CountPill count={activeCount} active />}
          </span>
        </Select.Value>
        <Select.Icon>
          <ChevronDown size={12} />
        </Select.Icon>
      </Select.Trigger>

      <Select.Portal>
        <Select.Content
          position="popper"
          sideOffset={4}
          className="z-50 min-w-[180px] rounded-lg border border-[var(--border-strong)] bg-panel shadow-xl overflow-hidden"
        >
          <Select.Viewport className="p-1">
            {TABS.map((t) => {
              const isActive = t.key === tab
              const count = counts[t.key]
              return (
                <Select.Item
                  key={t.key}
                  value={t.key}
                  className={cn(
                    'flex items-center justify-between gap-2 px-3 py-1.5 text-sm rounded-md cursor-pointer outline-none',
                    'data-[highlighted]:bg-hover',
                    isActive ? 'text-violet-300' : 'text-secondary'
                  )}
                >
                  <Select.ItemText>
                    <span className="flex items-center">
                      {t.label}
                      {count != null && <CountPill count={count} active={isActive} />}
                    </span>
                  </Select.ItemText>
                  <Select.ItemIndicator>
                    <Check size={12} />
                  </Select.ItemIndicator>
                </Select.Item>
              )
            })}
          </Select.Viewport>
        </Select.Content>
      </Select.Portal>
    </Select.Root>
  )
}

export function JobStatusTabs({ tab, onChange, counts, leftSlot, rightSlot }: JobStatusTabsProps) {
  const isMobile = useIsMobile()

  return (
    <div className="flex items-end gap-0.5 px-6 border-b border-[var(--border)]">
      {leftSlot && <div className="flex items-center pb-2 mr-4 shrink-0">{leftSlot}</div>}
      {isMobile ? (
        <div className="flex items-center py-2">
          <MobileTabSelect tab={tab} onChange={onChange} counts={counts} />
        </div>
      ) : (
        <DesktopTabs tab={tab} onChange={onChange} counts={counts} />
      )}
      {rightSlot && <div className="flex items-center pb-2 ml-auto shrink-0">{rightSlot}</div>}
    </div>
  )
}
