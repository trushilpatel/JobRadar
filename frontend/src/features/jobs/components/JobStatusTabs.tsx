import * as Select from '@radix-ui/react-select'
import { Check, ChevronDown } from 'lucide-react'
import { cn } from '@/lib/utils/utils'
import { useIsMobile } from '@/lib/hooks/use-media-query'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Checkbox } from '@/components/ui/checkbox'
import { SegmentedTabs, type SegmentedTabItem } from './SegmentedTabs'
import type { TabKey, TrackingStage } from '../hooks/useJobs'
import { JOB_STATUS, TRACKING_STAGES } from '@/config/constants/jobStatus'
import { COPY } from '@/config/copy'

const QUICK_TAB_KEYS = [JOB_STATUS.NEW, JOB_STATUS.SAVED] as const

const TAB_LABELS: Record<(typeof QUICK_TAB_KEYS)[number] | 'tracking', string> = {
  [JOB_STATUS.NEW]: COPY['jobs.tab.new'],
  [JOB_STATUS.SAVED]: COPY['jobs.tab.saved'],
  tracking: COPY['jobs.tab.tracking'],
}

const STAGE_LABELS: Record<TrackingStage, string> = {
  applied: COPY['jobs.tab.applied'],
  interviewing: COPY['jobs.tab.interviewing'],
  offer: COPY['jobs.tab.offer'],
  rejected: COPY['jobs.tab.rejected'],
}

const STAGE_DOT_CLASS: Record<TrackingStage, string> = {
  applied: 'bg-cyan',
  interviewing: 'bg-magenta',
  offer: 'bg-lime',
  rejected: 'bg-danger',
}

interface JobStatusTabsProps {
  tab: TabKey
  onChange: (tab: TabKey) => void
  counts: Partial<Record<TabKey, number>>
  /** Checked stages inside the Tracking tab's sub-menu. Defaults to all 4. */
  trackingStages?: TrackingStage[]
  onTrackingStagesChange?: (stages: TrackingStage[]) => void
  leftSlot?: React.ReactNode
  rightSlot?: React.ReactNode
}

function CountPill({ count, active }: { count: number; active: boolean }) {
  return (
    <span
      data-testid="job-tab-count"
      className={cn(
        'ml-1.5 text-xs px-1.5 py-0.5 rounded-full',
        // `violet` is a flat token, not a numbered shade scale —
        // `bg-violet-100`-style classes silently fail to generate.
        active
          ? 'bg-[color-mix(in_srgb,var(--violet)_16%,transparent)] text-violet'
          : 'bg-subtle text-muted'
      )}
    >
      {count > 999 ? '999+' : count}
    </span>
  )
}

function TrackingStageMenu({
  active,
  count,
  stages,
  onToggle,
  stageCounts,
  onSelect,
}: {
  active: boolean
  count?: number
  stages: TrackingStage[]
  onToggle: (stage: TrackingStage) => void
  stageCounts: Partial<Record<TrackingStage, number>>
  onSelect: () => void
}) {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          role="tab"
          aria-selected={active}
          onClick={onSelect}
          className={cn(
            'inline-flex h-8 items-center gap-1.5 whitespace-nowrap rounded-full px-3.5 text-sm font-medium transition-colors',
            'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet',
            active ? 'bg-panel text-primary shadow-sm' : 'text-muted hover:text-primary'
          )}
        >
          <span>{TAB_LABELS.tracking}</span>
          {count !== undefined && (
            <span
              className={cn(
                'font-mono text-[11px] tabular-nums',
                active ? 'text-violet' : 'text-muted'
              )}
            >
              {count > 999 ? '999+' : count}
            </span>
          )}
          <ChevronDown size={12} className={active ? 'text-primary' : 'text-muted'} />
        </button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-[220px] p-1 bg-panel border-[var(--border)] shadow-[0_8px_24px_rgba(0,0,0,0.4)]"
      >
        {TRACKING_STAGES.map((stage) => (
          <label
            key={stage}
            className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs text-secondary hover:bg-hover"
          >
            <Checkbox checked={stages.includes(stage)} onCheckedChange={() => onToggle(stage)} />
            <span className={cn('h-1.5 w-1.5 shrink-0 rounded-full', STAGE_DOT_CLASS[stage])} />
            <span className="flex-1">{STAGE_LABELS[stage]}</span>
            <span className="font-mono text-[11px] text-muted">{stageCounts[stage] ?? 0}</span>
          </label>
        ))}
      </PopoverContent>
    </Popover>
  )
}

function DesktopTabs({
  tab,
  onChange,
  counts,
  trackingStages,
  onTrackingStagesChange,
}: Omit<JobStatusTabsProps, 'leftSlot' | 'rightSlot'>) {
  const stages = trackingStages ?? [...TRACKING_STAGES]

  function toggleStage(stage: TrackingStage) {
    onTrackingStagesChange?.(
      stages.includes(stage) ? stages.filter((s) => s !== stage) : [...stages, stage]
    )
  }

  const items: SegmentedTabItem[] = [
    ...QUICK_TAB_KEYS.map((key) => ({ key, label: TAB_LABELS[key], count: counts[key] })),
    {
      key: 'tracking',
      label: TAB_LABELS.tracking,
      count: counts.tracking,
      render: ({ active, onSelect }) => (
        <TrackingStageMenu
          active={active}
          count={counts.tracking}
          stages={stages}
          onToggle={toggleStage}
          stageCounts={counts}
          onSelect={onSelect}
        />
      ),
    },
  ]

  return (
    <div data-testid="job-status-tabs-desktop">
      <SegmentedTabs
        items={items}
        value={tab}
        onChange={(key) => onChange(key as TabKey)}
        className="border-transparent bg-transparent p-0"
      />
    </div>
  )
}

const MOBILE_TABS = [...QUICK_TAB_KEYS, 'tracking'] as const

function MobileTabSelect({
  tab,
  onChange,
  counts,
}: Omit<JobStatusTabsProps, 'leftSlot' | 'rightSlot'>) {
  const activeLabel = TAB_LABELS[tab as (typeof MOBILE_TABS)[number]] ?? tab
  const activeCount = counts[tab]

  return (
    <Select.Root value={tab} onValueChange={(v) => onChange(v as TabKey)}>
      <Select.Trigger
        data-testid="job-status-tabs-mobile-trigger"
        aria-label="Select tab"
        className={cn(
          'flex items-center gap-2 px-3 py-1.5 rounded-lg border outline-none cursor-pointer transition-colors',
          // `violet` is a flat token, not a numbered shade scale —
          // `bg-violet-100`-style classes silently fail to generate.
          'text-sm font-medium text-violet bg-[color-mix(in_srgb,var(--violet)_10%,transparent)] border-[color-mix(in_srgb,var(--violet)_30%,transparent)]',
          'hover:brightness-110 focus-visible:ring-2 focus-visible:ring-violet'
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
            {MOBILE_TABS.map((key) => {
              const isActive = key === tab
              const count = counts[key]
              return (
                <Select.Item
                  key={key}
                  value={key}
                  className={cn(
                    'flex items-center justify-between gap-2 px-3 py-1.5 text-sm rounded-md cursor-pointer outline-none',
                    'data-[highlighted]:bg-hover',
                    isActive ? 'text-violet' : 'text-secondary'
                  )}
                >
                  <Select.ItemText>
                    <span className="flex items-center">
                      {TAB_LABELS[key]}
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

export function JobStatusTabs(props: JobStatusTabsProps) {
  const { leftSlot, rightSlot } = props
  const isMobile = useIsMobile()

  return (
    <div className="flex items-center gap-0.5 px-6 py-2.5">
      {leftSlot && <div className="flex items-center mr-4 shrink-0">{leftSlot}</div>}
      {isMobile ? (
        <div className="flex items-center">
          <MobileTabSelect {...props} />
        </div>
      ) : (
        <DesktopTabs {...props} />
      )}
      {rightSlot && <div className="flex items-center ml-auto shrink-0">{rightSlot}</div>}
    </div>
  )
}
