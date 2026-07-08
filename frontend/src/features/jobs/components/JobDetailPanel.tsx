import { useCallback, useEffect, useId, useRef, useState } from 'react'
import {
  X,
  ExternalLink,
  Wifi,
  Briefcase,
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
import { JOB_STATUS, SELECT_STATUSES, type SelectStatus } from '@/config/constants/jobStatus'
import { formatRelativeTime, formatExactDateTime, cn, formatSalary } from '@/lib/utils/utils'
import { CompanyAvatar } from '@/components/shared/CompanyAvatar'
import { STATUS_CONFIG } from './jobStatusConfig'
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
  mode?: JobDetailMode
  /** Optional slot rendered between tech stack and notes. Pages layer injects
   *  cross-feature content (e.g. JobResumeSection) here so this component
   *  stays within the features/jobs boundary. */
  resumeSection?: React.ReactNode
}

const NOTES_MAX = 5000
const NOTES_DEBOUNCE_MS = 800

// Meta-pill variants (mirror the mockup's PILL_BG map) expressed via brand
// tokens. `color-mix` arbitrary values keep the tint derived from the token so
// light/dark themes stay in sync without raw palette colors.
const META_PILL_BASE = 'text-xs px-2.5 py-1 rounded-lg whitespace-nowrap'
const META_PILL_NEUTRAL = cn(
  META_PILL_BASE,
  'text-secondary bg-subtle border border-[var(--border)]'
)
const META_PILL_SALARY = cn(
  META_PILL_BASE,
  'text-salary bg-[color-mix(in_srgb,var(--salary)_14%,transparent)] border border-[var(--border)]'
)
const META_PILL_OK = cn(
  META_PILL_BASE,
  'text-ok bg-[color-mix(in_srgb,var(--ok)_14%,transparent)] border border-[var(--border)]'
)
const META_PILL_WARN = cn(
  META_PILL_BASE,
  'text-warn bg-[color-mix(in_srgb,var(--warn)_14%,transparent)] border border-[var(--border)]'
)

type SaveState = 'idle' | 'saving' | 'saved' | 'error'

/**
 * Inline one-line segmented status control (mirrors the approved mockup's
 * `.statusseg` — 5 evenly-distributed segments) replacing the old dropdown
 * `StatusSelect`. Uses `role="radiogroup"`/`role="radio"` — this is a value
 * selector, not tab navigation, so it deliberately does NOT reuse
 * `SegmentedTabs` (which is `role="tablist"`/`role="tab"`).
 */
function StatusSegmentedControl({
  status,
  onChange,
}: {
  status: JobStatus
  onChange: (status: SelectStatus) => void
}) {
  return (
    <div
      role="radiogroup"
      aria-label="Job status"
      className="flex w-full gap-0.5 rounded-lg border border-[var(--border)] bg-subtle p-[3px]"
    >
      {SELECT_STATUSES.map((s) => {
        const active = s === status
        const cfg = STATUS_CONFIG[s]
        return (
          <button
            key={s}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onChange(s)}
            className={cn(
              'flex-1 truncate rounded-md px-1.5 py-1.5 text-center text-[11px] font-medium transition-colors',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet',
              active ? cfg.classes : 'text-muted hover:text-primary'
            )}
          >
            {cfg.label}
          </button>
        )
      })}
    </div>
  )
}

export function JobDetailPanel({
  job,
  onClose,
  onStatusChange,
  onToggleSave,
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
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

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

  // Clear save timer on unmount.
  useEffect(
    () => () => {
      if (saveTimerRef.current != null) clearTimeout(saveTimerRef.current)
    },
    []
  )

  const save = useCallback(
    async (text: string) => {
      if (text === lastSaved) return
      setSaveState('saving')
      try {
        await setJobNotes(job.id, text)
        setLastSaved(text)
        setSaveState('saved')
        if (saveTimerRef.current != null) clearTimeout(saveTimerRef.current)
        saveTimerRef.current = setTimeout(() => setSaveState('idle'), 2000)
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
    <div role="region" aria-label="Job details" className="flex flex-col flex-1 min-h-0 bg-panel">
      <div className="px-5 pt-4 pb-3 border-b border-[var(--border)]">
        {isRoute && (
          <button
            type="button"
            onClick={onClose}
            aria-label="Back to jobs"
            className="mb-3 flex min-h-[44px] items-center gap-1 px-2 py-2 text-sm text-secondary hover:text-primary rounded-lg hover:bg-hover transition-colors"
          >
            <ChevronLeft size={18} />
            Back
          </button>
        )}

        {/* Header band: avatar + company on the left, actions anchored top-right. */}
        <div className="flex items-start justify-between gap-3 mb-3">
          <div className="flex items-center gap-2.5 min-w-0">
            <CompanyAvatar name={companyName} size="md" />
            <span className="text-sm text-secondary font-medium truncate">{companyName}</span>
          </div>
          <div className="flex items-center gap-1 shrink-0">
            <button
              type="button"
              onClick={() => onToggleSave(job)}
              className={cn(
                'p-1.5 rounded-lg transition-colors',
                isSaved
                  ? 'text-salary hover:bg-hover'
                  : 'text-muted hover:text-salary hover:bg-hover'
              )}
              title={isSaved ? 'Remove from saved' : 'Save'}
              aria-label={isSaved ? 'Remove from saved' : 'Save job'}
              aria-pressed={isSaved}
            >
              <Bookmark size={14} fill={isSaved ? 'currentColor' : 'none'} />
            </button>
            {!isRoute && (
              <button
                onClick={onClose}
                aria-label="Close job details"
                className="p-1.5 text-muted hover:text-primary hover:bg-hover rounded-lg transition-colors"
              >
                <X size={15} />
              </button>
            )}
          </div>
        </div>

        {/* Display-font role title (mockup `.drawer h2`). */}
        <h2 className="font-display text-xl font-semibold text-primary leading-tight tracking-tight mb-3">
          {job.title}
        </h2>

        {/* Meta pills in two rows.
            Row 1: experience · salary · location.
            Row 2: posted · type · visa · citizenship/clearance. */}
        <div className="flex flex-col gap-2 mb-3">
          <div className="flex flex-wrap gap-2">
            {job.experience_years != null && (
              <span className={META_PILL_NEUTRAL}>
                {job.experience_years === 0 ? 'New grad' : `${job.experience_years}+ yrs`}
              </span>
            )}
            {salary && <span className={cn(META_PILL_SALARY, 'font-mono')}>{salary}</span>}
            {job.remote ? (
              <span className={cn(META_PILL_NEUTRAL, 'inline-flex items-center gap-1')}>
                <Wifi size={11} /> {job.location || 'Remote'}
              </span>
            ) : job.location ? (
              <span className={cn(META_PILL_NEUTRAL, 'inline-flex items-center gap-1')}>
                <MapPin size={11} /> {job.location}
              </span>
            ) : null}
          </div>
          {(job.added_at ||
            job.employ_type ||
            job.sponsorship ||
            job.requires_citizen ||
            job.security_clearance) && (
            <div className="flex flex-wrap gap-2">
              {job.added_at && (
                <span
                  className={META_PILL_NEUTRAL}
                  title={`Added ${formatExactDateTime(job.added_at)}`}
                >
                  Posted {formatRelativeTime(job.added_at)}
                </span>
              )}
              {job.employ_type && (
                <span className={cn(META_PILL_NEUTRAL, 'inline-flex items-center gap-1')}>
                  <Briefcase size={11} /> {job.employ_type.replaceAll('_', ' ')}
                </span>
              )}
              {job.sponsorship && (
                <span className={cn(META_PILL_OK, 'inline-flex items-center gap-1')}>
                  <Globe size={11} /> Visa sponsored
                </span>
              )}
              {job.requires_citizen && (
                <span className={cn(META_PILL_WARN, 'inline-flex items-center gap-1')}>
                  <BadgeCheck size={11} /> Citizenship required
                </span>
              )}
              {job.security_clearance && (
                <span className={cn(META_PILL_WARN, 'inline-flex items-center gap-1')}>
                  <ShieldAlert size={11} /> Clearance required
                </span>
              )}
            </div>
          )}
        </div>

        <div className="mb-2">
          <a
            href={job.url}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1.5 text-xs text-secondary hover:text-violet-bright bg-subtle border border-[var(--border-strong)] px-2.5 py-1.5 rounded-lg transition-colors"
          >
            <ExternalLink size={12} /> Open posting
          </a>
        </div>
        {/* Segmented status control shows in BOTH pane and route modes so
            mobile users can reach Interviewing / Offer / Rejected. */}
        <StatusSegmentedControl
          status={job.status}
          onChange={(status) => onStatusChange(job, status)}
        />
      </div>

      <div className={cn('flex-1 overflow-y-auto px-5 py-4 space-y-5', isRoute && 'pb-24')}>
        {(job.role_brief || job.llm_summary) && (
          <div>
            <h3 className="text-xs font-medium text-muted uppercase tracking-wider mb-2">
              The role
            </h3>
            <p className="text-sm text-secondary leading-relaxed">
              {job.role_brief ?? job.llm_summary}
            </p>
          </div>
        )}

        {job.responsibilities && job.responsibilities.length > 0 && (
          <div>
            <h3 className="text-xs font-medium text-muted uppercase tracking-wider mb-2">
              What you'll do
            </h3>
            <ul className="list-disc list-inside space-y-1">
              {job.responsibilities.map((r, i) => (
                <li key={i} className="text-sm text-secondary">
                  {r}
                </li>
              ))}
            </ul>
          </div>
        )}

        {techStack.length > 0 && (
          <div>
            <h3 className="text-xs font-medium text-muted uppercase tracking-wider mb-2">
              Tech stack
            </h3>
            <div className="flex flex-wrap gap-1.5">
              {techStack.map((t) => (
                <span
                  key={t}
                  className="text-xs bg-subtle text-secondary px-2 py-0.5 rounded-md font-mono"
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
            className="block text-xs font-medium text-muted uppercase tracking-wider mb-2"
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
            className="w-full resize-y rounded-lg border border-[var(--border)] bg-subtle px-3 py-2 text-sm text-primary placeholder:text-placeholder focus-visible:border-[var(--border-strong)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet"
          />
          <p
            id={notesStatusId}
            aria-live="polite"
            className={cn(
              'mt-1 text-[10px] transition-opacity',
              saveState === 'error' ? 'text-danger' : 'text-muted',
              saveState === 'idle' ? 'opacity-0' : 'opacity-100'
            )}
          >
            {saveStatusText}
          </p>
        </div>
      </div>

      {isRoute && (
        <div className="sticky bottom-0 flex gap-2 border-t border-[var(--border-strong)] bg-panel px-3 pt-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]">
          <button
            type="button"
            onClick={() => onToggleSave(job)}
            aria-pressed={isSaved}
            className={cn(
              'flex-1 min-h-[44px] rounded-lg px-4 py-2 text-sm font-semibold transition-colors flex items-center justify-center gap-1.5 border',
              isSaved
                ? 'text-salary bg-[color-mix(in_srgb,var(--salary)_16%,transparent)] border-[var(--border-strong)] hover:brightness-110'
                : 'text-primary bg-subtle border-[var(--border-strong)] hover:bg-hover'
            )}
          >
            <Bookmark size={14} fill={isSaved ? 'currentColor' : 'none'} />
            {isSaved ? 'Saved' : 'Save'}
          </button>
          <button
            type="button"
            onClick={() => onStatusChange(job, JOB_STATUS.APPLIED)}
            className="flex-1 min-h-[44px] rounded-lg bg-subtle px-4 py-2 text-sm font-semibold text-primary transition-colors hover:bg-hover border border-[var(--border-strong)]"
          >
            Applied
          </button>
        </div>
      )}
    </div>
  )
}
