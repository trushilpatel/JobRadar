import type { DateFilter, LocationFilter } from '../hooks/useJobs'

export const EXP_OPTIONS: { value: string; label: string }[] = [
  { value: '0-2', label: '0-2 yrs' },
  { value: '2-5', label: '2-5 yrs' },
  { value: '5+', label: '5+ yrs' },
]

export const DATE_OPTIONS: { value: Exclude<DateFilter, 'all'>; label: string }[] = [
  { value: '24h', label: '24h' },
  { value: '7d', label: '7d' },
  { value: '30d', label: '30d' },
]

export const LOCATION_OPTIONS: {
  value: LocationFilter
  label: string
}[] = [
  { value: 'remote', label: 'Remote' },
  { value: 'hybrid', label: 'Hybrid' },
  { value: 'onsite', label: 'On-site' },
]

export const VISA_OPTIONS: { value: 'true' | 'false'; label: string }[] = [
  { value: 'true', label: 'Sponsorship offered' },
  { value: 'false', label: 'No sponsorship' },
]

/**
 * Adapts a FilterPopover's multi-select `next: string[]` callback into a
 * single-select value with "click active option again -> fallback" semantics
 * (mirrors the old FilterPill toggle behavior: click the active option
 * clears it back to the default, click a different option switches directly
 * to it).
 */
export function pickSingleSelectValue<T extends string>(
  next: string[],
  current: T,
  fallback: T
): T {
  const chosen = next.find((v) => v !== current)
  return (chosen as T) ?? fallback
}
