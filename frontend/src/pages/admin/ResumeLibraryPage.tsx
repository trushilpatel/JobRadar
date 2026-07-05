import { useState } from 'react'
import { Upload } from 'lucide-react'
import { useResumes, ResumeCard, ResumeStorageMeter, UploadResumeDialog } from '@/features/resumes'
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
import { RESUME_LIMIT_PER_USER } from '@/features/resumes'

export function ResumeLibraryPage() {
  const { data, isLoading, isError, refetch } = useResumes()
  const resumes = data ?? []
  const atLimit = resumes.length >= RESUME_LIMIT_PER_USER
  const [uploadOpen, setUploadOpen] = useState(false)

  return (
    <div className="flex-1 flex flex-col min-h-0 overflow-auto">
      <div className="px-6 py-6 space-y-6 w-full">
        {/* Header */}
        <div className="flex items-start justify-between gap-6 flex-wrap">
          <div>
            <h1 className="text-2xl font-semibold text-primary">Resumes</h1>
            <p className="text-sm text-muted mt-1">
              Upload and manage your resume files. Download or delete anytime.
            </p>
          </div>
          <div className="flex items-center gap-3">
            <ResumeStorageMeter count={resumes.length} />
            <button
              type="button"
              disabled={atLimit}
              onClick={() => setUploadOpen(true)}
              className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-lg bg-violet hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed text-white"
            >
              <Upload size={14} />
              Upload resume
            </button>
          </div>
        </div>

        {atLimit && (
          <div className="rounded-lg border border-red-500/30 bg-red-500/5 px-4 py-2.5 text-sm text-red-300">
            Resume limit reached ({RESUME_LIMIT_PER_USER}). Delete an existing resume to add a new
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
                  <div className="h-3 w-1/2 rounded bg-slate-800/70" />
                </div>
              ))}
            </div>
          ) : isError ? (
            <div className="flex flex-col items-center justify-center py-20 gap-3 text-secondary">
              <p className="text-sm">Failed to load. Please try again.</p>
              <button
                type="button"
                onClick={() => refetch()}
                className="text-xs text-violet hover:opacity-80 underline"
              >
                Retry
              </button>
            </div>
          ) : resumes.length === 0 ? (
            <EmptyState onUpload={() => setUploadOpen(true)} />
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
              {resumes.map((r) => (
                <ResumeCard key={r.id} resume={r} />
              ))}
            </div>
          )}
        </ErrorBoundary>
      </div>
      <UploadResumeDialog open={uploadOpen} onOpenChange={setUploadOpen} />
    </div>
  )
}

function EmptyState({ onUpload }: { onUpload: () => void }) {
  return (
    <div className="rounded-xl border border-dashed border-[var(--border)] px-6 py-16 text-center">
      <div className="text-base text-primary mb-1">No resumes yet</div>
      <div className="text-sm text-muted mb-5">Upload your first resume to get started.</div>
      <button
        type="button"
        onClick={onUpload}
        className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-lg bg-violet text-white hover:opacity-90"
      >
        <Upload size={14} />
        Upload first resume
      </button>
    </div>
  )
}
