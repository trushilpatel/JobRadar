import { cn } from '@/lib/utils/utils'
import type { JobStatus } from '@/types'
import { STATUS_CONFIG } from './jobStatusConfig'

interface JobStatusPillProps {
  status: JobStatus
  className?: string
}

/**
 * Read-only status pill — dot + label on brand tokens. Shares the single
 * STATUS_CONFIG color source with StatusSelect so the table row pill and the
 * drawer dropdown can never drift. Used by the jobs table.
 */
export function JobStatusPill({ status, className }: JobStatusPillProps) {
  const cfg = STATUS_CONFIG[status] ?? STATUS_CONFIG.new
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11.5px] font-semibold whitespace-nowrap',
        cfg.classes,
        className
      )}
    >
      <span className="w-1.5 h-1.5 rounded-full bg-current" aria-hidden="true" />
      {cfg.label}
    </span>
  )
}
