import { cn } from '@/lib/utils/utils'
import type { ApplyQueueStatus } from '../api/apply-queue.api'

// Status → brand-token styling. Mirrors the color intent of
// features/jobs/components/jobStatusConfig.ts (applied=cyan,
// interviewing=magenta) so the two features never present the same status
// with different colors — but this feature cannot import that file directly
// (cross-feature imports are forbidden; see features/CLAUDE.md), so the
// three-status subset is re-declared here as the single source of truth for
// apply-queue pill colors.
const STATUS_CONFIG: Record<ApplyQueueStatus, { label: string; classes: string }> = {
  saved: {
    label: 'Saved',
    classes:
      'text-salary bg-[color-mix(in_srgb,var(--salary)_14%,transparent)] border-[var(--border)]',
  },
  applied: {
    label: 'Applied',
    classes: 'text-cyan bg-[color-mix(in_srgb,var(--cyan)_14%,transparent)] border-[var(--border)]',
  },
  interviewing: {
    label: 'Interviewing',
    classes:
      'text-magenta bg-[color-mix(in_srgb,var(--magenta)_14%,transparent)] border-[var(--border)]',
  },
}

interface ApplyQueueStatusPillProps {
  status: ApplyQueueStatus
  className?: string
}

export function ApplyQueueStatusPill({ status, className }: ApplyQueueStatusPillProps) {
  const cfg = STATUS_CONFIG[status]
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
