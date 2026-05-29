import { Link } from 'react-router-dom'
import { FileText, Paperclip, CheckCircle2 } from 'lucide-react'
import { cn, formatRelativeTime } from '@/lib/utils/utils'
import { routes } from '@/lib/routes'
import type { Resume } from '@/features/resumes/types'

const AXIS_TONE: Record<string, string> = {
  role: 'bg-violet-500/10 text-violet-300',
  seniority: 'bg-sky-500/10 text-sky-300',
  industry: 'bg-rose-500/10 text-rose-300',
  stack: 'bg-emerald-500/10 text-emerald-300',
  geo: 'bg-amber-500/10 text-amber-300',
}

export function ResumeCard({ resume }: { resume: Resume }) {
  return (
    <Link
      to={routes.resumeDetail(resume.id)}
      className={cn(
        'group block rounded-xl border border-white/[0.06] bg-[#141420] p-4',
        'hover:border-violet-500/40 hover:bg-[#171723] transition-colors'
      )}
    >
      <div className="flex items-start justify-between gap-3 mb-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-white font-medium truncate">
            <FileText size={14} className="text-slate-500 shrink-0" />
            <span className="truncate">{resume.name}</span>
          </div>
          <div className="text-[11px] text-slate-500 mt-0.5 font-mono">{resume.label}</div>
        </div>
        <div className="text-[10px] text-slate-500 shrink-0 whitespace-nowrap">
          v{resume.version_count}
        </div>
      </div>

      <div className="flex flex-wrap gap-1 mb-4">
        {resume.tags.map((t, i) => (
          <span
            key={i}
            className={cn(
              'inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium',
              AXIS_TONE[t.axis] ?? 'bg-white/[0.05] text-slate-400'
            )}
          >
            {t.value}
          </span>
        ))}
      </div>

      <div className="flex items-center justify-between text-xs text-slate-400">
        <div className="flex items-center gap-3">
          <span className="inline-flex items-center gap-1">
            <Paperclip size={11} />
            {resume.attached_job_count}
          </span>
          <span className="inline-flex items-center gap-1">
            <CheckCircle2 size={11} />
            {resume.submitted_count}
          </span>
        </div>
        <span className="text-slate-500">Updated {formatRelativeTime(resume.updated_at)}</span>
      </div>
    </Link>
  )
}
