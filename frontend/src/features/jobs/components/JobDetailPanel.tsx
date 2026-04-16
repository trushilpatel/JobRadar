import {
  X,
  ExternalLink,
  Wifi,
  Briefcase,
  DollarSign,
  Bookmark,
  BookmarkCheck,
  Send,
  Trash2,
  TrendingUp,
  Award,
} from 'lucide-react'
import type { Job } from '@/types'
import { formatRelativeTime } from '@/lib/utils'
import { formatSalary } from '@/services/jobs.service'
import { CompanyAvatar } from '@/components/shared/CompanyAvatar'

interface JobDetailPanelProps {
  job: Job
  onClose: () => void
  onSave: (job: Job) => void
  onApply: (job: Job) => void
  onMarkInterviewing: (job: Job) => void
  onMarkOffer: (job: Job) => void
  onDismiss: (job: Job) => void
}

export function JobDetailPanel({
  job,
  onClose,
  onSave,
  onApply,
  onMarkInterviewing,
  onMarkOffer,
  onDismiss,
}: JobDetailPanelProps) {
  const salary = formatSalary(job.salary_range)
  const techStack: string[] = Array.isArray(job.tech_stack) ? job.tech_stack : []
  const companyName = job.company?.name ?? job.company_id ?? ''

  return (
    <div className="flex flex-col flex-1 min-h-0 bg-[#0d0d14]">
      {/* Header */}
      <div className="px-5 pt-4 pb-3 border-b border-white/[0.07]">
        {/* Top-right action buttons */}
        <div className="flex items-center justify-end gap-1 mb-3">
          <button
            onClick={() => onDismiss(job)}
            className="p-1.5 text-slate-400 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors"
            title="Dismiss"
          >
            <Trash2 size={14} />
          </button>
          <button
            onClick={onClose}
            className="p-1.5 text-slate-400 hover:text-white hover:bg-white/[0.06] rounded-lg transition-colors"
          >
            <X size={15} />
          </button>
        </div>

        {/* Avatar + Title + Subtitle */}
        <div className="flex items-start gap-3 mb-3">
          <CompanyAvatar name={companyName} size="md" />
          <div className="min-w-0">
            <p className="text-base font-semibold text-white leading-tight">{job.title}</p>
            <p className="text-sm text-slate-400 mt-0.5">
              {companyName} · {job.location ?? 'Location unknown'}
            </p>
          </div>
        </div>

        {/* Badges */}
        <div className="flex flex-wrap gap-2 mb-3">
          {job.remote && (
            <span className="flex items-center gap-1 text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 px-2.5 py-1 rounded-lg">
              <Wifi size={11} /> Remote
            </span>
          )}
          {salary && (
            <span className="flex items-center gap-1 text-xs text-amber-300 bg-amber-500/10 border border-amber-500/20 px-2.5 py-1 rounded-lg">
              <DollarSign size={11} /> {salary}
            </span>
          )}
          {job.employ_type && (
            <span className="flex items-center gap-1 text-xs text-slate-400 bg-white/[0.05] border border-white/[0.07] px-2.5 py-1 rounded-lg">
              <Briefcase size={11} /> {job.employ_type.replace('_', ' ')}
            </span>
          )}
          {job.experience_years != null && (
            <span className="text-xs text-slate-400 bg-white/[0.05] border border-white/[0.07] px-2.5 py-1 rounded-lg">
              {job.experience_years}+ yrs exp
            </span>
          )}
        </div>

        {/* Actions */}
        <div className="flex items-center gap-2 flex-wrap">
          <a
            href={job.url}
            target="_blank"
            rel="noreferrer"
            className="flex items-center gap-1.5 text-xs text-slate-300 hover:text-violet-300 bg-white/5 border border-white/10 px-2.5 py-1.5 rounded-lg transition-colors"
          >
            <ExternalLink size={12} /> Open posting
          </a>

          {/* Save button — show for new/saved */}
          {(job.status === 'new' || job.status === 'saved') &&
            (job.saved ? (
              <button
                onClick={() => onSave(job)}
                className="flex items-center gap-1.5 text-xs text-violet-300 bg-violet-500/10 border border-violet-500/20 hover:bg-violet-500/20 px-2.5 py-1.5 rounded-lg transition-colors"
              >
                <BookmarkCheck size={12} /> Saved
              </button>
            ) : (
              <button
                onClick={() => onSave(job)}
                className="flex items-center gap-1.5 text-xs text-white bg-violet-600 hover:bg-violet-500 px-2.5 py-1.5 rounded-lg transition-colors"
              >
                <Bookmark size={12} /> Save
              </button>
            ))}

          {/* Advancement buttons */}
          {(job.status === 'new' || job.status === 'saved') && (
            <button
              onClick={() => onApply(job)}
              className="flex items-center gap-1.5 text-xs text-white bg-emerald-600 hover:bg-emerald-500 px-2.5 py-1.5 rounded-lg transition-colors"
            >
              <Send size={12} /> Mark applied
            </button>
          )}
          {job.status === 'applied' && (
            <button
              onClick={() => onMarkInterviewing(job)}
              className="flex items-center gap-1.5 text-xs text-white bg-blue-600 hover:bg-blue-500 px-2.5 py-1.5 rounded-lg transition-colors"
            >
              <TrendingUp size={12} /> Mark interviewing
            </button>
          )}
          {job.status === 'interviewing' && (
            <button
              onClick={() => onMarkOffer(job)}
              className="flex items-center gap-1.5 text-xs text-white bg-amber-600 hover:bg-amber-500 px-2.5 py-1.5 rounded-lg transition-colors"
            >
              <Award size={12} /> Mark offer
            </button>
          )}
        </div>
      </div>

      {/* Body */}
      <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
        <p className="text-xs text-slate-500">Added {formatRelativeTime(job.added_at)}</p>

        {(job.role_brief || job.llm_summary) && (
          <div>
            <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">
              Summary
            </h3>
            <p className="text-sm text-slate-300 leading-relaxed">
              {job.role_brief ?? job.llm_summary}
            </p>
          </div>
        )}

        {job.responsibilities && job.responsibilities.length > 0 && (
          <div>
            <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">
              Responsibilities
            </h3>
            <ul className="list-disc list-inside space-y-1">
              {job.responsibilities.map((r, i) => (
                <li key={i} className="text-sm text-slate-300">
                  {r}
                </li>
              ))}
            </ul>
          </div>
        )}

        {techStack.length > 0 && (
          <div>
            <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">
              Tech stack
            </h3>
            <div className="flex flex-wrap gap-1.5">
              {techStack.map((t) => (
                <span
                  key={t}
                  className="text-xs bg-white/[0.07] text-slate-300 px-2 py-0.5 rounded-md font-mono"
                >
                  {t}
                </span>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
