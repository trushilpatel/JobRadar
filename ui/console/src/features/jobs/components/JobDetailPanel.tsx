import {
  X,
  ExternalLink,
  Wifi,
  Briefcase,
  Trash2,
  MapPin,
  ShieldAlert,
  Globe,
  BadgeCheck,
  ChevronLeft,
} from 'lucide-react'
import type { Job, JobStatus } from '@/types'
import { JOB_STATUS } from '@/constants/jobStatus'
import { formatRelativeTime, cn } from '@/lib/utils'
import { formatSalary } from '@/services/jobs.service'
import { CompanyAvatar } from '@/components/shared/CompanyAvatar'
import { StatusSelect } from '@/features/jobs/components/StatusSelect'

export type JobDetailMode = 'pane' | 'route'

interface JobDetailPanelProps {
  job: Job
  onClose: () => void
  onStatusChange: (job: Job, status: Exclude<JobStatus, 'dismissed'>) => void
  onDismiss: (job: Job) => void
  mode?: JobDetailMode
}

export function JobDetailPanel({
  job,
  onClose,
  onStatusChange,
  onDismiss,
  mode = 'pane',
}: JobDetailPanelProps) {
  const salary = formatSalary(job.salary_range)
  const techStack: string[] = Array.isArray(job.tech_stack) ? job.tech_stack : []
  const companyName = job.company?.name ?? job.company_id ?? ''
  const isRoute = mode === 'route'

  return (
    <div className="flex flex-col flex-1 min-h-0 bg-[#0d0d14]">
      <div className="px-5 pt-4 pb-3 border-b border-white/[0.07]">
        <div
          className={cn(
            'flex items-center mb-3',
            isRoute ? 'justify-between' : 'justify-end gap-1'
          )}
        >
          {isRoute && (
            <button
              type="button"
              onClick={onClose}
              aria-label="Back"
              className="flex min-h-[44px] items-center gap-1 px-2 py-2 text-sm text-slate-300 hover:text-white rounded-lg hover:bg-white/[0.06] transition-colors"
            >
              <ChevronLeft size={18} />
              Back
            </button>
          )}
          <div className="flex items-center gap-1">
            <button
              onClick={() => onDismiss(job)}
              className="p-1.5 text-slate-400 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors"
              title="Dismiss"
              aria-label="Dismiss"
            >
              <Trash2 size={14} />
            </button>
            {!isRoute && (
              <button
                onClick={onClose}
                aria-label="Close"
                className="p-1.5 text-slate-400 hover:text-white hover:bg-white/[0.06] rounded-lg transition-colors"
              >
                <X size={15} />
              </button>
            )}
          </div>
        </div>

        <div className="flex items-start gap-3 mb-3">
          <CompanyAvatar name={companyName} size="md" />
          <div className="min-w-0">
            <p className="text-base font-semibold text-white leading-tight">{job.title}</p>
            <p className="text-sm text-slate-400 mt-0.5">{companyName}</p>
          </div>
        </div>

        <div className="flex flex-wrap gap-2 mb-3">
          {job.experience_years != null && (
            <span className="text-xs text-slate-400 bg-white/[0.05] border border-white/[0.07] px-2.5 py-1 rounded-lg">
              {job.experience_years}+ yrs exp
            </span>
          )}
          {job.remote ? (
            <span className="flex items-center gap-1 text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 px-2.5 py-1 rounded-lg">
              <Wifi size={11} /> Remote
            </span>
          ) : job.location ? (
            <span className="flex items-center gap-1 text-xs text-slate-400 bg-white/[0.05] border border-white/[0.07] px-2.5 py-1 rounded-lg">
              <MapPin size={11} /> {job.location}
            </span>
          ) : null}
          {salary && (
            <span className="text-xs text-amber-300 bg-amber-500/10 border border-amber-500/20 px-2.5 py-1 rounded-lg">
              {salary}
            </span>
          )}
          {job.employ_type && (
            <span className="flex items-center gap-1 text-xs text-slate-400 bg-white/[0.05] border border-white/[0.07] px-2.5 py-1 rounded-lg">
              <Briefcase size={11} /> {job.employ_type.replaceAll('_', ' ')}
            </span>
          )}
          {job.sponsorship && (
            <span className="flex items-center gap-1 text-xs text-blue-300 bg-blue-500/10 border border-blue-500/20 px-2.5 py-1 rounded-lg">
              <Globe size={11} /> Sponsors visa
            </span>
          )}
          {job.requires_citizen && (
            <span className="flex items-center gap-1 text-xs text-orange-300 bg-orange-500/10 border border-orange-500/20 px-2.5 py-1 rounded-lg">
              <BadgeCheck size={11} /> Citizen required
            </span>
          )}
          {job.security_clearance && (
            <span className="flex items-center gap-1 text-xs text-red-300 bg-red-500/10 border border-red-500/20 px-2.5 py-1 rounded-lg">
              <ShieldAlert size={11} /> Clearance required
            </span>
          )}
        </div>

        <div className="flex items-center gap-2 flex-wrap">
          <a
            href={job.url}
            target="_blank"
            rel="noreferrer"
            className="flex items-center gap-1.5 text-xs text-slate-300 hover:text-violet-300 bg-white/5 border border-white/10 px-2.5 py-1.5 rounded-lg transition-colors"
          >
            <ExternalLink size={12} /> Open posting
          </a>
          {!isRoute && (
            <StatusSelect status={job.status} onChange={(status) => onStatusChange(job, status)} />
          )}
        </div>
      </div>

      <div className={cn('flex-1 overflow-y-auto px-5 py-4 space-y-5', isRoute && 'pb-24')}>
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

      {isRoute && (
        <div className="sticky bottom-0 flex gap-2 border-t border-white/[0.08] bg-[#0d0d14] p-3">
          <button
            type="button"
            onClick={() => onStatusChange(job, JOB_STATUS.SAVED)}
            className="flex-1 min-h-[44px] rounded-lg bg-violet-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-violet-500"
          >
            Save
          </button>
          <button
            type="button"
            onClick={() => onStatusChange(job, JOB_STATUS.APPLIED)}
            className="flex-1 min-h-[44px] rounded-lg bg-white/[0.06] px-4 py-2 text-sm font-medium text-slate-200 transition-colors hover:bg-white/[0.12]"
          >
            Applied
          </button>
          <button
            type="button"
            onClick={() => onDismiss(job)}
            className="flex-1 min-h-[44px] rounded-lg bg-red-600/80 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-600"
          >
            Dismiss
          </button>
        </div>
      )}
    </div>
  )
}
