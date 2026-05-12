import * as Select from '@radix-ui/react-select'
import { Check, ChevronDown } from 'lucide-react'
import { cn } from '@/lib/utils'
import { SELECT_STATUSES, type SelectStatus } from '@/constants/jobStatus'
import type { JobStatus } from '@/types'

const STATUS_CONFIG: Record<JobStatus, { label: string; classes: string }> = {
  new: {
    label: 'New',
    classes: 'text-slate-300 bg-white/[0.06] border-white/[0.12]',
  },
  saved: {
    label: 'Saved',
    classes: 'text-violet-300 bg-violet-500/10 border-violet-500/20',
  },
  applied: {
    label: 'Applied',
    classes: 'text-emerald-300 bg-emerald-500/10 border-emerald-500/20',
  },
  interviewing: {
    label: 'Interviewing',
    classes: 'text-blue-300 bg-blue-500/10 border-blue-500/20',
  },
  offer: {
    label: 'Offer',
    classes: 'text-amber-300 bg-amber-500/10 border-amber-500/20',
  },
  dismissed: {
    label: 'Dismissed',
    classes: 'text-slate-400 bg-white/[0.04] border-white/[0.08]',
  },
  rejected: {
    label: 'Rejected',
    classes: 'text-red-300 bg-red-500/10 border-red-500/20',
  },
}

type SettableStatus = SelectStatus

const ALL_STATUSES: SettableStatus[] = [...SELECT_STATUSES]

interface StatusSelectProps {
  status: JobStatus
  onChange: (status: SettableStatus) => void
  disabled?: boolean
}

export function StatusSelect({ status, onChange, disabled }: StatusSelectProps) {
  const current = STATUS_CONFIG[status]

  return (
    <Select.Root
      value={status}
      onValueChange={(v) => onChange(v as SettableStatus)}
      disabled={disabled}
    >
      <Select.Trigger
        className={cn(
          'flex items-center gap-1.5 text-xs px-2.5 py-1.5 rounded-lg border transition-colors outline-none cursor-pointer',
          'hover:brightness-110 disabled:opacity-50 disabled:cursor-not-allowed',
          current.classes
        )}
      >
        <Select.Value />
        <Select.Icon>
          <ChevronDown size={11} />
        </Select.Icon>
      </Select.Trigger>

      <Select.Portal>
        <Select.Content
          position="popper"
          sideOffset={4}
          className="z-50 min-w-[140px] rounded-lg border border-white/[0.1] bg-[#13131f] shadow-xl overflow-hidden"
        >
          <Select.Viewport className="p-1">
            {ALL_STATUSES.map((s) => {
              const cfg = STATUS_CONFIG[s]
              return (
                <Select.Item
                  key={s}
                  value={s}
                  className={cn(
                    'flex items-center justify-between gap-2 px-3 py-1.5 text-xs rounded-md cursor-pointer outline-none',
                    'data-[highlighted]:bg-white/[0.06]',
                    cfg.classes.split(' ')[0]
                  )}
                >
                  <Select.ItemText>{cfg.label}</Select.ItemText>
                  <Select.ItemIndicator>
                    <Check size={11} />
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
