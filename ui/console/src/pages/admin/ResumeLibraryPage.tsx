import { Link } from 'react-router-dom'
import { Upload, Plus, FileText } from 'lucide-react'
import { toast } from 'sonner'
import { routes } from '@/config/routes'
import { useResumes, ResumeCard, ResumeStorageMeter } from '@/features/resumes'
import { RESUME_LIMIT_PER_USER } from '@/features/resumes'

export function ResumeLibraryPage() {
  const { data, isLoading } = useResumes()
  const resumes = data ?? []
  const atLimit = resumes.length >= RESUME_LIMIT_PER_USER

  return (
    <div className="flex-1 flex flex-col min-h-0 overflow-auto">
      <div className="px-6 py-6 space-y-6 w-full">
        {/* Header */}
        <div className="flex items-start justify-between gap-6 flex-wrap">
          <div>
            <h1 className="text-2xl font-semibold text-white">Resumes</h1>
            <p className="text-sm text-slate-400 mt-1">
              Maintain a library of tailored resumes. Attach versions to saved jobs, track draft →
              final → submitted state per application.
            </p>
          </div>
          <div className="flex items-center gap-3">
            <ResumeStorageMeter count={resumes.length} />
            <button
              disabled={atLimit}
              onClick={() => toast.info('Upload flow (mockup) — would open file picker')}
              className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-lg bg-violet-600 hover:bg-violet-500 disabled:bg-white/[0.04] disabled:text-slate-500 text-white"
            >
              <Upload size={14} />
              Upload resume
            </button>
            <button
              disabled={atLimit}
              onClick={() => toast.info('New blank resume (mockup)')}
              className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-lg bg-white/[0.04] hover:bg-white/[0.08] disabled:opacity-50 text-slate-200"
            >
              <Plus size={14} />
              New
            </button>
          </div>
        </div>

        {atLimit && (
          <div className="rounded-lg border border-red-500/30 bg-red-500/5 px-4 py-2.5 text-sm text-red-300">
            Resume limit reached ({RESUME_LIMIT_PER_USER}). Archive an older resume to add a new
            one.
          </div>
        )}

        {/* Grid */}
        {isLoading ? (
          <div className="text-sm text-slate-400">Loading…</div>
        ) : resumes.length === 0 ? (
          <EmptyState />
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
            {resumes.map((r) => (
              <ResumeCard key={r.id} resume={r} />
            ))}
          </div>
        )}

        {/* Footer hint linking to Apply Queue — the other surface that uses resumes */}
        <div className="pt-2">
          <Link
            to={routes.applyQueue()}
            className="text-xs text-slate-400 hover:text-white inline-flex items-center gap-2"
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
    <div className="rounded-xl border border-dashed border-white/[0.08] px-6 py-16 text-center">
      <div className="text-base text-white mb-1">No resumes yet</div>
      <div className="text-sm text-slate-400 mb-5">
        Upload a base resume to start tailoring per role / company / job.
      </div>
      <button className="inline-flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-lg bg-violet-600 hover:bg-violet-500 text-white">
        <Upload size={14} />
        Upload first resume
      </button>
    </div>
  )
}
