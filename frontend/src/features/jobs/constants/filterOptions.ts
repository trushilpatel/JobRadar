import type { DateFilter, LocationFilter } from '../hooks/useJobs'

export const EXP_OPTIONS = ['0-2', '2-5', '5+'] as const

export const DATE_OPTIONS: { value: Exclude<DateFilter, 'all'>; label: string }[] = [
  { value: '24h', label: '24h' },
  { value: '7d', label: '7d' },
  { value: '30d', label: '30d' },
]

export const LOCATION_OPTIONS: {
  value: LocationFilter
  label: string
  color?: 'violet' | 'emerald'
}[] = [
  { value: 'remote', label: 'Remote', color: 'emerald' },
  { value: 'onsite', label: 'On-site', color: 'violet' },
]
