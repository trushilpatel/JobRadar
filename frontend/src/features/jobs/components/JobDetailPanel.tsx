import { useState, useRef, useEffect } from 'react'
import {
  X,
  ExternalLink,
  Wifi,
  Briefcase,
  DollarSign,
  Bookmark,
  BookmarkCheck,
  Trash2,
  ChevronDown,
  MapPin,
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

const STATUS_LABELS: Record<string, string> = {
  new: 'New',
  saved: 'Saved',
  applied: 'Applied',
  interviewing: 'Interviewing',
  offer: 'Offer',
}

function getNextSteps(
  status: string
): { label: string; action: 'apply' | 'interviewing' | 'offer' }[] {
  switch (status) {
    case 'new':
    case 'saved':
      return [{ label: 'Mark Applied', action: 'apply' }]
    case 'applied':
      return [{ label: 'Mark Interviewing', action: 'interviewing' }]
    case 'interviewing':
      return [{ label: 'Mark Offer', action: 'offer' }]
    default:
      return []
  }
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
  const nextSteps = getNextSteps(job.status)

  const [dropdownOpen, setDropdownOpen] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!dropdownOpen) return
    function onDown(e: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node))
        setDropdownOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [dropdownOpen])

  function handleNextStep(action: 'apply' | 'interviewing' | 'offer') {
    setDropdownOpen(false)
    if (action === 'apply') onApply(job)
    else if (action === 'interviewing') onMarkInterviewing(job)
    else if (action === 'offer') onMarkOffer(job)
  }

  return (
    <div className="flex flex-col flex-1 min-h-0 bg-[#0d0d14]">
      {/* Header */}
      <div className="px-5 pt-4 pb-3 border-b border-white/[0.07]">
        {/* Top-right: dismiss + close */}
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
            <p className="text-sm text-slate-400 mt-0.5">{companyName}</p>
          </div>
        </div>

        {/* Row 1: exp · location · salary · employ type */}
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
            <span className="flex items-center gap-1 text-xs text-amber-300 bg-amber-500/10 border border-amber-500/20 px-2.5 py-1 rounded-lg">
              <DollarSign size={11} /> {salary}
            </span>
          )}
          {job.employ_type && (
            <span className="flex items-center gap-1 text-xs text-slate-400 bg-white/[0.05] border border-white/[0.07] px-2.5 py-1 rounded-lg">
              <Briefcase size={11} /> {job.employ_type.replace('_', ' ')}
            </span>
          )}
        </div>

        {/* Row 2: open posting · save · status dropdown */}
        <div className="flex items-center gap-2 flex-wrap">
          <a
            href={job.url}
            target="_blank"
            rel="noreferrer"
            className="flex items-center gap-1.5 text-xs text-slate-300 hover:text-violet-300 bg-white/5 border border-white/10 px-2.5 py-1.5 rounded-lg transition-colors"
          >
            <ExternalLink size={12} /> Open posting
          </a>

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

          {nextSteps.length > 0 && (
            <div className="relative" ref={dropdownRef}>
              <button
                onClick={() => setDropdownOpen((o) => !o)}
                className="flex items-center gap-1.5 text-xs text-slate-300 bg-white/[0.05] border border-white/[0.1] hover:bg-white/[0.09] px-2.5 py-1.5 rounded-lg transition-colors"
              >
                {STATUS_LABELS[job.status] ?? job.status}
                <ChevronDown size={11} />
              </button>
              {dropdownOpen && (
                <div className="absolute top-full left-0 mt-1 z-30 min-w-[160px] bg-[#13131e] border border-white/[0.1] rounded-[10px] shadow-[0_8px_24px_rgba(0,0,0,0.4)] overflow-hidden py-1">
                  {nextSteps.map((step) => (
                    <button
                      key={step.action}
                      onClick={() => handleNextStep(step.action)}
                      className="w-full text-left px-3 py-2 text-xs text-slate-300 hover:bg-white/[0.06] hover:text-white transition-colors"
                    >
                      {step.label}
                    </button>
                  ))}
                </div>
              )}
            </div>
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
