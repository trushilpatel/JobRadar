import { useCallback, useEffect, useId, useRef, useState } from 'react'
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
  Bookmark,
  ChevronLeft,
} from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import type { Job, JobStatus } from '@/types'
import { JOB_STATUS } from '@/config/constants/jobStatus'
import { formatRelativeTime, cn, formatSalary } from '@/lib/utils/utils'
import { CompanyAvatar } from '@/components/shared/CompanyAvatar'
import { StatusSelect } from './StatusSelect'
import { setJobNotes } from '../api/jobs.api'

export type JobDetailMode = 'pane' | 'route'

interface JobDetailPanelProps {
  job: Job
  onClose: () => void
  onStatusChange: (job: Job, status: Exclude<JobStatus, 'dismissed' | 'saved'>) => void
  /**
   * Toggle the bookmark/saved flag. ORTHOGONAL to status — a saved job stays
   * in its current workflow stage. Bookmarking has its own dedicated icon.
   */
  onToggleSave: (job: Job) => void
  onDismiss: (job: Job) => void
  mode?: JobDetailMode
  /** Optional slot rendered between tech stack and notes. Pages layer injects
   *  cross-feature content (e.g. JobResumeSection) here so this component
   *  stays within the features/jobs boundary. */
  resumeSection?: React.ReactNode
}

const NOTES_MAX = 5000
const NOTES_DEBOUNCE_MS = 800

type SaveState = 'idle' | 'saving' | 'saved' | 'error'

export function JobDetailPanel({
  job,
  onClose,
  onStatusChange,
  onToggleSave,
  onDismiss,
  mode = 'pane',
  resumeSection,
}: JobDetailPanelProps) {
  const isSaved = job.user_state?.saved_at != null
  const salary = formatSalary(job.salary_range)
  const techStack: string[] = Array.isArray(job.tech_stack) ? job.tech_stack : []
  const companyName = job.company?.name ?? job.company_id ?? ''
  const isRoute = mode === 'route'

  const queryClient = useQueryClient()
  const notesLabelId = useId()
  const notesStatusId = useId()

  // Notes draft + persistence state. `lastSaved` tracks what the server has
  // (so we don't re-PATCH unchanged content) — it's state, not a ref,
  // because the React Compiler lint rule disallows ref writes during render.
  const [notes, setNotes] = useState(job.notes ?? '')
  const [saveState, setSaveState] = useState<SaveState>('idle')
  const [lastSaved, setLastSaved] = useState(job.notes ?? '')
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Re-seed the draft when switching to a different job. prev-state-during-
  // render is React's recommended alternative to a setState-in-useEffect.
  const [prevJobId, setPrevJobId] = useState(job.id)
  if (prevJobId !== job.id) {
    setPrevJobId(job.id)
    setNotes(job.notes ?? '')
    setLastSaved(job.notes ?? '')
    setSaveState('idle')
  }

  // Cancel any pending debounce timer on unmount.
  useEffect(() => {
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [])

  const save = useCallback(
    async (text: string) => {
      if (text === lastSaved) return
      setSaveState('saving')
      try {
        await setJobNotes(job.id, text)
        setLastSaved(text)
        setSaveState('saved')
        queryClient.invalidateQueries({ queryKey: ['jobs'] })
      } catch {
        setSaveState('error')
        toast.error('Failed to save notes')
      }
    },
    [job.id, queryClient, lastSaved]
  )

  function scheduleDebouncedSave(text: string) {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      void save(text)
    }, NOTES_DEBOUNCE_MS)
  }

  function handleNotesChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
    const next = e.target.value.slice(0, NOTES_MAX)
    setNotes(next)
    scheduleDebouncedSave(next)
  }

  function handleNotesBlur() {
    if (debounceRef.current) {
      clearTimeout(debounceRef.current)
      debounceRef.current = null
    }
    void save(notes)
  }

  const saveStatusText =
    saveState === 'saving'
      ? 'Saving…'
      : saveState === 'saved'
        ? 'Saved'
        : saveState === 'error'
          ? 'Failed to save'
          : ''

  return (
    <div
      role="region"
      aria-label="Job details"
      className="flex flex-col flex-1 min-h-0 bg-[#0d0d14]"
    >
      <div className="px-5 pt-4 pb-3 border-b border-white/[0.07]">
        {isRoute && (
          <button
            type="button"
            onClick={onClose}
            aria-label="Back to jobs"
            className="mb-3 flex min-h-[44px] items-center gap-1 px-2 py-2 text-sm text-slate-300 hover:text-white rounded-lg hover:bg-white/[0.06] transition-colors"
          >
            <ChevronLeft size={18} />
            Back
          </button>
        )}

        {/* Header band: avatar + title (wrappable) on the left, actions anchored top-right. */}
        <div className="flex items-start gap-3 mb-3">
          <CompanyAvatar name={companyName} size="md" />
          <div className="min-w-0 flex-1">
            <p className="text-base font-semibold text-white leading-tight">{job.title}</p>
            <p className="text-sm text-slate-400 mt-0.5">
              {companyName}
              <span className="text-slate-600"> · added {formatRelativeTime(job.added_at)}</span>
            </p>
          </div>
          <div className="flex items-center gap-1 shrink-0">
            <button
              type="button"
              onClick={() => onToggleSave(job)}
              className={cn(
                'p-1.5 rounded-lg transition-colors',
                isSaved
                  ? 'text-amber-400 hover:bg-amber-500/10'
                  : 'text-slate-400 hover:text-amber-300 hover:bg-white/[0.06]'
              )}
              title={isSaved ? 'Remove from saved' : 'Save'}
              aria-label={isSaved ? 'Remove from saved' : 'Save job'}
              aria-pressed={isSaved}
            >
              <Bookmark size={14} fill={isSaved ? 'currentColor' : 'none'} />
            </button>
            <button
              onClick={() => onDismiss(job)}
              className="p-1.5 text-slate-400 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors"
              title="Dismiss"
              aria-label="Dismiss job"
            >
              <Trash2 size={14} />
            </button>
            {!isRoute && (
              <button
                onClick={onClose}
                aria-label="Close job details"
                className="p-1.5 text-slate-400 hover:text-white hover:bg-white/[0.06] rounded-lg transition-colors"
              >
                <X size={15} />
              </button>
            )}
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
          {/* StatusSelect now shows in BOTH pane and route modes so mobile
              users can reach Interviewing / Offer / Rejected. */}
          <StatusSelect status={job.status} onChange={(status) => onStatusChange(job, status)} />
        </div>
      </div>

      <div className={cn('flex-1 overflow-y-auto px-5 py-4 space-y-5', isRoute && 'pb-24')}>
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

        {resumeSection}

        <div>
          <label
            id={notesLabelId}
            htmlFor="job-notes"
            className="block text-xs font-medium text-slate-400 uppercase tracking-wider mb-2"
          >
            Your notes
          </label>
          <textarea
            id="job-notes"
            value={notes}
            onChange={handleNotesChange}
            onBlur={handleNotesBlur}
            rows={4}
            maxLength={NOTES_MAX}
            placeholder="Private notes about this job (only you can see these)…"
            aria-labelledby={notesLabelId}
            aria-describedby={notesStatusId}
            className="w-full resize-y rounded-lg border border-white/[0.07] bg-white/[0.03] px-3 py-2 text-sm text-slate-200 placeholder:text-slate-600 focus-visible:border-violet-500/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400"
          />
          <p
            id={notesStatusId}
            aria-live="polite"
            className={cn(
              'mt-1 text-[10px] transition-opacity',
              saveState === 'error' ? 'text-red-400' : 'text-slate-500',
              saveState === 'idle' ? 'opacity-0' : 'opacity-100'
            )}
          >
            {saveStatusText}
          </p>
        </div>
      </div>

      {isRoute && (
        <div className="sticky bottom-0 flex gap-2 border-t border-white/[0.08] bg-[#0d0d14] px-3 pt-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]">
          <button
            type="button"
            onClick={() => onToggleSave(job)}
            aria-pressed={isSaved}
            className={cn(
              'flex-1 min-h-[44px] rounded-lg px-4 py-2 text-sm font-medium transition-colors flex items-center justify-center gap-1.5',
              isSaved
                ? 'bg-amber-500/20 text-amber-300 hover:bg-amber-500/30'
                : 'bg-violet-600 text-white hover:bg-violet-500'
            )}
          >
            <Bookmark size={14} fill={isSaved ? 'currentColor' : 'none'} />
            {isSaved ? 'Saved' : 'Save'}
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
