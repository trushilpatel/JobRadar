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
    // `violet` is a flat token (see tailwind.config.js), not a numbered shade
    // scale — `bg-violet-100`-style classes silently fail to generate.
    // color-mix() against the bare token is this project's established
    // pattern for a tinted background (see jobStatusConfig.ts).
    <span className="inline-flex items-center gap-1 rounded-full border border-[color-mix(in_srgb,var(--violet)_40%,transparent)] bg-[color-mix(in_srgb,var(--violet)_14%,transparent)] py-1 pl-2.5 pr-1.5 text-xs text-violet">
      {label}
      <button
        type="button"
        onClick={onRemove}
        aria-label={`Remove ${label}`}
        className="flex items-center justify-center rounded-full p-0.5 text-violet hover:bg-[color-mix(in_srgb,var(--violet)_25%,transparent)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet"
      >
        <X size={11} />
      </button>
    </span>
  )
}
