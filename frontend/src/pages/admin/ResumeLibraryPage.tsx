import { Link } from 'react-router-dom'
import { Upload, Plus, FileText } from 'lucide-react'
import { routes } from '@/config/routes'
import { useResumes, ResumeCard, ResumeStorageMeter } from '@/features/resumes'
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
import { RESUME_LIMIT_PER_USER } from '@/features/resumes'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

export function ResumeLibraryPage() {
  const { data, isLoading, isError, refetch } = useResumes()
  const resumes = data ?? []
  const atLimit = resumes.length >= RESUME_LIMIT_PER_USER

  return (
    <div className="flex-1 flex flex-col min-h-0 overflow-auto">
      <div className="px-6 py-6 space-y-6 w-full">
        {/* Header */}
        <div className="flex items-start justify-between gap-6 flex-wrap">
          <div>
            <h1 className="text-2xl font-semibold text-primary">Resumes</h1>
            <p className="text-sm text-muted mt-1">
              Maintain a library of tailored resumes. Attach versions to saved jobs, track draft →
              final → submitted state per application.
            </p>
          </div>
          <div className="flex items-center gap-3">
            <ResumeStorageMeter count={resumes.length} />
            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <button
                    type="button"
                    disabled
                    className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-lg bg-violet-600 hover:bg-violet-500 disabled:bg-subtle disabled:text-muted text-white opacity-50 cursor-not-allowed"
                  >
                    <Upload size={14} />
                    Upload resume
                  </button>
                </span>
              </TooltipTrigger>
              <TooltipContent>Coming soon — file upload is in progress</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <button
                    type="button"
                    disabled
                    className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-lg bg-subtle hover:bg-hover disabled:opacity-50 text-primary opacity-50 cursor-not-allowed"
                  >
                    <Plus size={14} />
                    New
                  </button>
                </span>
              </TooltipTrigger>
              <TooltipContent>Coming soon — file upload is in progress</TooltipContent>
            </Tooltip>
          </div>
        </div>

        {atLimit && (
          <div className="rounded-lg border border-red-500/30 bg-red-500/5 px-4 py-2.5 text-sm text-red-300">
            Resume limit reached ({RESUME_LIMIT_PER_USER}). Archive an older resume to add a new
            one.
          </div>
        )}

        {/* Grid */}
        <ErrorBoundary>
          {isLoading ? (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
              {Array.from({ length: 6 }).map((_, i) => (
                <div
                  key={i}
                  className="rounded-xl border border-[var(--border)] bg-panel p-5 animate-pulse"
                >
                  <div className="h-4 w-2/3 rounded bg-slate-800 mb-3" />
                  <div className="h-3 w-1/2 rounded bg-slate-800/70 mb-6" />
                  <div className="flex gap-2 mb-4">
                    <div className="h-5 w-16 rounded-full bg-slate-800/70" />
                    <div className="h-5 w-16 rounded-full bg-slate-800/70" />
                  </div>
                  <div className="h-3 w-1/3 rounded bg-slate-800/50" />
                </div>
              ))}
            </div>
          ) : isError ? (
            <div className="flex flex-col items-center justify-center py-20 gap-3 text-secondary">
              <p className="text-sm">Failed to load. Please try again.</p>
              <button
                type="button"
                onClick={() => refetch()}
                className="text-xs text-violet-400 hover:text-violet-300 underline"
              >
                Retry
              </button>
            </div>
          ) : resumes.length === 0 ? (
            <EmptyState />
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
              {resumes.map((r) => (
                <ResumeCard key={r.id} resume={r} />
              ))}
            </div>
          )}
        </ErrorBoundary>

        {/* Footer hint linking to Apply Queue — the other surface that uses resumes */}
        <div className="pt-2">
          <Link
            to={routes.applyQueue()}
            className="text-xs text-muted hover:text-primary inline-flex items-center gap-2"
          >
            <FileText size={12} />
            See apply queue → saved jobs with attached resumes & state
          </Link>
        </div>
      </div>
    </div>
  )
}

function EmptyState() {
  return (
    <div className="rounded-xl border border-dashed border-[var(--border)] px-6 py-16 text-center">
      <div className="text-base text-primary mb-1">No resumes yet</div>
      <div className="text-sm text-muted mb-5">
        Upload a base resume to start tailoring per role / company / job.
      </div>
      <Tooltip>
        <TooltipTrigger asChild>
          <span>
            <button
              type="button"
              disabled
              className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-lg bg-violet-600 text-white opacity-50 cursor-not-allowed"
            >
              <Upload size={14} />
              Upload first resume
            </button>
          </span>
        </TooltipTrigger>
        <TooltipContent>Coming soon — file upload is in progress</TooltipContent>
      </Tooltip>
    </div>
  )
}
