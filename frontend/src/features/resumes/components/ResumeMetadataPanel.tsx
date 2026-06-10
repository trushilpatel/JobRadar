import { cn } from '@/lib/utils/utils'
import type { Resume } from '../types'
import { AXIS_TONE_WITH_RING } from '../constants'

const AXIS_LABEL: Record<string, string> = {
  role: 'Role',
  seniority: 'Seniority',
  industry: 'Industry',
  stack: 'Stack',
  geo: 'Geo',
}

export function ResumeMetadataPanel({ resume }: { resume: Resume }) {
  return (
    <div className="rounded-xl border border-[var(--border)] bg-panel p-5">
      <div className="flex items-start justify-between gap-4 mb-4">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold text-primary truncate">{resume.name}</h1>
          <div className="text-xs text-muted font-mono mt-1">{resume.label}</div>
        </div>
        <div className="text-right shrink-0">
          <div className="text-[11px] text-muted uppercase tracking-wide">Versions</div>
          <div className="text-lg text-primary tabular-nums">{resume.version_count}</div>
        </div>
      </div>

      <div className="grid grid-cols-3 gap-3 mb-5">
        <Stat label="Attached" value={resume.attached_job_count} />
        <Stat label="Submitted" value={resume.submitted_count} />
        <Stat
          label="Conversion"
          value={
            resume.attached_job_count > 0
              ? `${Math.round((resume.submitted_count / resume.attached_job_count) * 100)}%`
              : '—'
          }
        />
      </div>

      <div>
        <div className="text-[11px] text-muted uppercase tracking-wide mb-2">Tags</div>
        <div className="flex flex-wrap gap-1.5">
          {resume.tags.map((t, i) => (
            <span
              key={i}
              className={cn(
                'inline-flex items-center px-2 py-0.5 rounded text-[11px] font-medium ring-1',
                AXIS_TONE_WITH_RING[t.axis] ?? 'bg-subtle text-muted ring-[var(--border)]'
              )}
              title={AXIS_LABEL[t.axis] ?? t.axis}
            >
              {t.value}
            </span>
          ))}
        </div>
      </div>
    </div>
  )
}

function Stat({ label, value }: { label: string; value: number | string }) {
  return (
    <div className="rounded-lg bg-subtle border border-[var(--border)] px-3 py-2">
      <div className="text-[10px] text-muted uppercase tracking-wide">{label}</div>
      <div className="text-base text-primary tabular-nums mt-0.5">{value}</div>
    </div>
  )
}
