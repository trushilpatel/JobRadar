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
    return <div className="px-6 py-6 text-sm text-slate-400">Loading…</div>
  }
  if (!data?.resume) {
    return (
      <div className="px-6 py-6">
        <Link to={routes.resumeLibrary()} className="text-sm text-violet-300 hover:underline">
          ← Back
        </Link>
        <div className="mt-4 text-sm text-slate-400">Resume not found.</div>
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
      <div className="flex items-center justify-between px-6 py-3 border-b border-white/[0.07] shrink-0">
        <div className="flex items-center gap-3 min-w-0">
          <Link
            to={routes.resumeDetail(resume.id)}
            className="inline-flex items-center gap-1.5 text-sm text-slate-400 hover:text-white shrink-0"
          >
            <ChevronLeft size={14} />
            Back
          </Link>
          <div className="h-4 w-px bg-white/10" />
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <FileEdit size={12} className="text-slate-500" />
              <span className="text-sm text-white font-medium truncate">{resume.name}</span>
              <span className="text-[11px] text-slate-500 tabular-nums">
                editing v{activeVersion?.version_number ?? '?'}
              </span>
              {currentAttachment && <ResumeStateBadge state={currentAttachment.state} />}
            </div>
            {splitView && (
              <div className="text-[11px] text-slate-500 mt-0.5 truncate">
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
              className="text-xs px-2.5 py-1.5 rounded bg-white/[0.04] hover:bg-white/[0.08] text-slate-300"
            >
              Apply queue →
            </Link>
          )}
        </div>
      </div>

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
