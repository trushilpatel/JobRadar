import { useMemo } from 'react'
import { Link, useParams, useSearchParams, useNavigate } from 'react-router-dom'
import { ChevronLeft, CheckCircle2, FileEdit } from 'lucide-react'
import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import { cn } from '@/lib/utils/utils'
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
import { routes } from '@/config/routes'
import {
  useResume,
  ResumeContentPane,
  JobContextPanel,
  ResumeStateBadge,
  getMockJob,
  resumeQueryKeys,
} from '@/features/resumes'

export function ResumeEditorPage() {
  const { id } = useParams<{ id: string }>()
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const jobId = params.get('job')

  const { data, isLoading } = useResume(id)
  const job = useMemo(() => (jobId ? getMockJob(jobId) : undefined), [jobId])

  if (isLoading) {
    return (
      <div className="space-y-6 animate-pulse p-6">
        <div className="h-6 w-48 rounded bg-slate-800" />
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <div className="h-64 rounded-xl bg-slate-800/60" />
          <div className="lg:col-span-2 h-64 rounded-xl bg-slate-800/60" />
        </div>
        <div className="h-48 rounded-xl bg-slate-800/40" />
      </div>
    )
  }
  if (!data?.resume) {
    return (
      <div className="px-6 py-6">
        <Link to={routes.resumeLibrary()} className="text-sm text-violet-300 hover:underline">
          ← Back
        </Link>
        <div className="mt-4 text-sm text-secondary">Resume not found.</div>
      </div>
    )
  }

  if (jobId && !job) {
    return (
      <div className="flex items-center justify-center h-full text-secondary text-sm">
        Job not found (ID: {jobId})
      </div>
    )
  }

  const { resume, versions, attachments } = data
  const activeVersion = versions.find((v) => v.id === resume.active_version_id)
  const currentAttachment = jobId ? attachments.find((a) => a.job_id === jobId) : undefined

  const markFinal = () => {
    toast.success('Marked as final — ready to apply from queue')
    qc.invalidateQueries({ queryKey: resumeQueryKeys.applyQueue('all') })
    navigate(routes.applyQueue())
  }

  const splitView = !!job

  return (
    <div className="flex-1 flex flex-col min-h-0 overflow-hidden">
      {/* Toolbar */}
      <div className="flex items-center justify-between px-6 py-3 border-b border-[var(--border)] shrink-0">
        <div className="flex items-center gap-3 min-w-0">
          <Link
            to={routes.resumeDetail(resume.id)}
            className="inline-flex items-center gap-1.5 text-sm text-secondary hover:text-primary shrink-0"
          >
            <ChevronLeft size={14} />
            Back
          </Link>
          <div className="h-4 w-px bg-[var(--border)]" />
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <FileEdit size={12} className="text-muted" />
              <span className="text-sm text-white font-medium truncate">{resume.name}</span>
              <span className="text-[11px] text-muted tabular-nums">
                editing v{activeVersion?.version_number ?? '?'}
              </span>
              {currentAttachment && <ResumeStateBadge state={currentAttachment.state} />}
            </div>
            {splitView && (
              <div className="text-[11px] text-muted mt-0.5 truncate">
                Tailoring for: {job!.title} · {job!.company}
              </div>
            )}
          </div>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          {splitView && currentAttachment?.state !== 'final' && (
            <button
              onClick={markFinal}
              className="inline-flex items-center gap-1 text-xs px-3 py-1.5 rounded bg-emerald-600 hover:bg-emerald-500 text-white"
            >
              <CheckCircle2 size={12} />
              Mark as final
            </button>
          )}
          {splitView && (
            <Link
              to={routes.applyQueue()}
              className="text-xs px-2.5 py-1.5 rounded bg-subtle hover:bg-hover text-secondary"
            >
              Apply queue →
            </Link>
          )}
        </div>
      </div>

      {/* Solo edit info bar — shown when not in split-view (no ?job= param) */}
      {!splitView && resume && (
        <div className="flex items-center gap-3 px-4 py-2.5 border-b border-[var(--border)] text-xs text-secondary bg-panel shrink-0">
          <span className="text-secondary font-medium">{resume.name}</span>
          <span className="text-placeholder">·</span>
          <span>v{activeVersion?.version_number ?? 1}</span>
          {activeVersion?.change_summary && (
            <>
              <span className="text-placeholder">·</span>
              <span className="truncate max-w-xs">{activeVersion.change_summary}</span>
            </>
          )}
        </div>
      )}

      {/* Body */}
      <ErrorBoundary>
        <div
          className={cn(
            'flex-1 min-h-0 overflow-hidden p-4',
            splitView ? 'grid grid-cols-1 lg:grid-cols-2 gap-4' : 'w-full'
          )}
        >
          {activeVersion && (
            <ResumeContentPane
              content={activeVersion.content_text}
              className="h-full"
              onSave={() => {
                /* mockup */
              }}
            />
          )}
          {splitView && job && <JobContextPanel job={job} />}
        </div>
      </ErrorBoundary>
    </div>
  )
}
