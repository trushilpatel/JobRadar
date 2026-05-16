import { useMemo } from 'react'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from '@/components/ui/command'
import type { Job } from '@/types'
import type { TabKey } from '@/features/jobs/hooks/useJobs'
import { JOB_STATUS } from '@/constants/jobStatus'

export interface JobCommandPaletteProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  recentJobs?: Job[]
  onGoToTab?: (tab: TabKey) => void
  onSelectJob?: (job: Job) => void
  onResetFilters?: () => void
  onOpenHelp?: () => void
}

const TAB_ENTRIES: { tab: TabKey; label: string; shortcut: string }[] = [
  { tab: JOB_STATUS.NEW, label: 'Go to New', shortcut: 'g n' },
  { tab: JOB_STATUS.SAVED, label: 'Go to Saved', shortcut: 'g s' },
  { tab: JOB_STATUS.APPLIED, label: 'Go to Applied', shortcut: 'g a' },
  { tab: JOB_STATUS.INTERVIEWING, label: 'Go to Interviewing', shortcut: 'g i' },
  { tab: JOB_STATUS.OFFER, label: 'Go to Offer', shortcut: 'g o' },
  { tab: JOB_STATUS.REJECTED, label: 'Go to Rejected', shortcut: 'g r' },
]

export function JobCommandPalette({
  open,
  onOpenChange,
  recentJobs = [],
  onGoToTab,
  onSelectJob,
  onResetFilters,
  onOpenHelp,
}: JobCommandPaletteProps) {
  const runAndClose = useMemo(
    () => (fn?: () => void) => () => {
      fn?.()
      onOpenChange(false)
    },
    [onOpenChange]
  )

  return (
    <CommandDialog open={open} onOpenChange={onOpenChange}>
      <CommandInput placeholder="Type a command or search jobs…" />
      <CommandList>
        <CommandEmpty>No results.</CommandEmpty>

        <CommandGroup heading="Navigate">
          {TAB_ENTRIES.map((entry) => (
            <CommandItem key={entry.tab} onSelect={runAndClose(() => onGoToTab?.(entry.tab))}>
              {entry.label}
              <CommandShortcut>{entry.shortcut}</CommandShortcut>
            </CommandItem>
          ))}
        </CommandGroup>

        <CommandGroup heading="Actions">
          <CommandItem onSelect={runAndClose(() => onResetFilters?.())}>
            Reset filters
            <CommandShortcut>⌘⇧R</CommandShortcut>
          </CommandItem>
          <CommandItem onSelect={runAndClose(() => onOpenHelp?.())}>
            Show keyboard shortcuts
            <CommandShortcut>?</CommandShortcut>
          </CommandItem>
        </CommandGroup>

        {recentJobs.length > 0 && (
          <CommandGroup heading="Jobs in view">
            {recentJobs.slice(0, 10).map((job) => (
              <CommandItem
                key={job.id}
                value={`${job.company?.name ?? ''} ${job.title}`}
                onSelect={runAndClose(() => onSelectJob?.(job))}
              >
                <span className="truncate">
                  <span className="text-slate-400">{job.company?.name ?? '—'}</span>
                  <span className="mx-1 text-slate-500">·</span>
                  <span>{job.title}</span>
                </span>
              </CommandItem>
            ))}
          </CommandGroup>
        )}
      </CommandList>
    </CommandDialog>
  )
}
