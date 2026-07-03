import type { JobStatus } from '@/types'

// Status → brand-token styling. Colors mirror the approved mockup's STATUS map
// (new=violet, saved=salary, applied=cyan, interviewing=magenta, offer=lime,
// rejected=danger) expressed via the design tokens in index.css rather than raw
// Tailwind palette utilities. This is the single source of truth for job
// status colors in the jobs feature — both StatusSelect (dropdown) and
// JobStatusPill (table) read from it so they can never drift.
export const STATUS_CONFIG: Record<JobStatus, { label: string; classes: string }> = {
  new: {
    label: 'New',
    classes: 'text-violet-bright bg-[var(--accent-dim)] border-[var(--border-strong)]',
  },
  // 'saved' kept in the config only to satisfy the JobStatus Record<>.
  // It's never offered in the dropdown — SELECT_STATUSES excludes it.
  // The bookmark icon next to the dropdown handles save/unsave instead.
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
  offer: {
    label: 'Offer',
    classes: 'text-lime bg-[color-mix(in_srgb,var(--lime)_14%,transparent)] border-[var(--border)]',
  },
  dismissed: {
    label: 'Dismissed',
    classes: 'text-muted bg-subtle border-[var(--border)]',
  },
  rejected: {
    label: 'Rejected',
    classes:
      'text-danger bg-[color-mix(in_srgb,var(--danger)_14%,transparent)] border-[var(--border)]',
  },
}
