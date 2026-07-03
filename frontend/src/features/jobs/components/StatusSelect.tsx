import * as Select from '@radix-ui/react-select'
import { Check, ChevronDown } from 'lucide-react'
import { cn } from '@/lib/utils/utils'
import { SELECT_STATUSES, type SelectStatus } from '@/config/constants/jobStatus'
import type { JobStatus } from '@/types'
import { STATUS_CONFIG } from './jobStatusConfig'

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
          className="z-50 min-w-[140px] rounded-lg border border-[var(--border-strong)] bg-panel shadow-xl overflow-hidden"
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
                    'data-[highlighted]:bg-hover',
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
