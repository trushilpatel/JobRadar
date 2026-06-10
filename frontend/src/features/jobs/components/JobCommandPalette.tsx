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
import { Kbd, KbdGroup } from '@/components/ui/kbd'
import { formatKeys } from '@/lib/commands/formatKeys'
import type { CommandCategory } from '@/lib/commands/types'
import type { Job } from '@/types'
import type { TabKey } from '../hooks/useJobs'
import { JOB_COMMANDS, type JobCommandId } from '../commands/jobCommands'

export interface JobCommandPaletteProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  recentJobs?: Job[]
  onGoToTab?: (tab: TabKey) => void
  onSelectJob?: (job: Job) => void
  onResetFilters?: () => void
  onOpenHelp?: () => void
}

const CATEGORY_HEADINGS: Record<CommandCategory, string> = {
  navigation: 'Navigation',
  tabs: 'Navigate',
  status: 'Status',
  search: 'Actions',
}

const TAB_FOR_COMMAND: Partial<Record<JobCommandId, TabKey>> = {
  'jobs.tab.go-new': 'new',
  'jobs.tab.go-saved': 'saved',
  'jobs.tab.go-applied': 'applied',
  'jobs.tab.go-interviewing': 'interviewing',
  'jobs.tab.go-offer': 'offer',
  'jobs.tab.go-rejected': 'rejected',
}

// The actionable subset of commands the palette surfaces, in render order.
// Each maps to a prop callback resolved at select time; labels + key glyphs
// derive from the registry so they never drift.
const SURFACED: JobCommandId[] = [
  'jobs.tab.go-new',
  'jobs.tab.go-saved',
  'jobs.tab.go-applied',
  'jobs.tab.go-interviewing',
  'jobs.tab.go-offer',
  'jobs.tab.go-rejected',
  'jobs.nav.reset-filters',
  'jobs.nav.open-help',
]

const PALETTE_CATEGORIES: CommandCategory[] = ['tabs', 'search']

function CommandKeys({ keys }: { keys: readonly string[] }) {
  const primary = keys[0]
  if (!primary) return null
  const groups = formatKeys(primary)
  return (
    <CommandShortcut>
      <span className="inline-flex items-center gap-1">
        {groups.map((group, gi) => (
          <KbdGroup key={gi}>
            {group.map((glyph, ki) => (
              <Kbd key={`${gi}-${ki}`}>{glyph}</Kbd>
            ))}
          </KbdGroup>
        ))}
      </span>
    </CommandShortcut>
  )
}

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

  // Resolve the callback a palette row should invoke for a given command id.
  const callbackFor = (id: JobCommandId): (() => void) | undefined => {
    const tab = TAB_FOR_COMMAND[id]
    if (tab) return onGoToTab ? () => onGoToTab(tab) : undefined
    if (id === 'jobs.nav.reset-filters') return onResetFilters
    if (id === 'jobs.nav.open-help') return onOpenHelp
    return undefined
  }

  return (
    <CommandDialog open={open} onOpenChange={onOpenChange}>
      <CommandInput placeholder="Type a command or search jobs…" />
      <CommandList>
        <CommandEmpty>No results.</CommandEmpty>

        {PALETTE_CATEGORIES.map((category) => {
          const rows = SURFACED.filter((id) => JOB_COMMANDS[id].category === category)
          if (rows.length === 0) return null
          return (
            <CommandGroup key={category} heading={CATEGORY_HEADINGS[category]}>
              {rows.map((id) => {
                const def = JOB_COMMANDS[id]
                return (
                  <CommandItem key={id} onSelect={runAndClose(callbackFor(id))}>
                    {def.label}
                    <CommandKeys keys={def.keys} />
                  </CommandItem>
                )
              })}
            </CommandGroup>
          )
        })}

        {recentJobs.length > 0 && (
          <CommandGroup heading="Jobs in view">
            {recentJobs.slice(0, 10).map((job) => (
              <CommandItem
                key={job.id}
                value={`${job.company?.name ?? ''} ${job.title}`}
                onSelect={runAndClose(() => onSelectJob?.(job))}
              >
                <span className="truncate">
                  <span className="text-muted">{job.company?.name ?? '—'}</span>
                  <span className="mx-1 text-muted">·</span>
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
