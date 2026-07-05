import { MapPin, Wifi } from 'lucide-react'

export interface JobLocationCellProps {
  remote: boolean
  location: string | null
}

/**
 * Shared location-cell rendering for any table that lists jobs — extracted
 * from features/jobs/jobs-columns.tsx so features/apply-queue can render an
 * identical cell without a cross-feature import. Remote jobs get a Wifi badge
 * (falling back to the literal "Remote" label when no location string is
 * present); on-site jobs get a MapPin + location text; jobs with neither
 * render nothing — per UX direction, absence of data means the field
 * disappears rather than showing a placeholder dash.
 */
export function JobLocationCell({ remote, location }: JobLocationCellProps) {
  if (remote)
    return (
      <span className="inline-flex items-center gap-1.5 text-[13px] text-remote">
        <Wifi size={11} /> {location || 'Remote'}
      </span>
    )
  if (location)
    return (
      <span className="inline-flex items-center gap-1.5 text-[13px] text-secondary">
        <MapPin size={11} className="text-muted" /> {location}
      </span>
    )
  return null
}
