import { X } from 'lucide-react'

export interface FilterChipProps {
  label: string
  onRemove: () => void
}

/**
 * Pure presentational removable pill for the active-filter chip row in
 * JobFilters. Zero business logic — data in, render out, callback out.
 */
export function FilterChip({ label, onRemove }: FilterChipProps) {
  return (
    <span className="inline-flex items-center gap-1 rounded-full border border-violet-500/30 bg-violet-500/15 py-1 pl-2.5 pr-1.5 text-xs text-violet-200">
      {label}
      <button
        type="button"
        onClick={onRemove}
        aria-label={`Remove ${label}`}
        className="flex items-center justify-center rounded-full p-0.5 text-violet-300 hover:bg-violet-500/25 hover:text-violet-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500"
      >
        <X size={11} />
      </button>
    </span>
  )
}
