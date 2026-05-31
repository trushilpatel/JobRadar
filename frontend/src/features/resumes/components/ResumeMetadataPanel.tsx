import { cn } from '@/lib/utils/utils'
import type { Resume } from '../types'

const AXIS_TONE: Record<string, string> = {
  role: 'bg-violet-500/10 text-violet-300 ring-violet-500/20',
  seniority: 'bg-sky-500/10 text-sky-300 ring-sky-500/20',
  industry: 'bg-rose-500/10 text-rose-300 ring-rose-500/20',
  stack: 'bg-emerald-500/10 text-emerald-300 ring-emerald-500/20',
  geo: 'bg-amber-500/10 text-amber-300 ring-amber-500/20',
}

const AXIS_LABEL: Record<string, string> = {
  role: 'Role',
  seniority: 'Seniority',
  industry: 'Industry',
  stack: 'Stack',
  geo: 'Geo',
}

export function ResumeMetadataPanel({ resume }: { resume: Resume }) {
  return (
    <div className="rounded-xl border border-white/[0.06] bg-[#141420] p-5">
      <div className="flex items-start justify-between gap-4 mb-4">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold text-white truncate">{resume.name}</h1>
          <div className="text-xs text-slate-500 font-mono mt-1">{resume.label}</div>
        </div>
        <div className="text-right shrink-0">
          <div className="text-[11px] text-slate-500 uppercase tracking-wide">Versions</div>
          <div className="text-lg text-white tabular-nums">{resume.version_count}</div>
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
        <div className="text-[11px] text-slate-500 uppercase tracking-wide mb-2">Tags</div>
        <div className="flex flex-wrap gap-1.5">
          {resume.tags.map((t, i) => (
            <span
              key={i}
              className={cn(
                'inline-flex items-center px-2 py-0.5 rounded text-[11px] font-medium ring-1',
                AXIS_TONE[t.axis] ?? 'bg-white/[0.05] text-slate-400 ring-white/10'
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
    <div className="rounded-lg bg-white/[0.02] border border-white/[0.04] px-3 py-2">
      <div className="text-[10px] text-slate-500 uppercase tracking-wide">{label}</div>
      <div className="text-base text-white tabular-nums mt-0.5">{value}</div>
    </div>
  )
}
