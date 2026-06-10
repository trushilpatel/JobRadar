import { useState } from 'react'
import { Link, useParams, useNavigate } from 'react-router-dom'
import { ChevronLeft, Edit3, Archive, Download } from 'lucide-react'
import { toast } from 'sonner'
import { routes } from '@/config/routes'
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
import {
  useResume,
  ResumeMetadataPanel,
  ResumeVersionList,
  ResumeContentPane,
  AttachedJobsList,
  VERSION_LIMIT_PER_RESUME,
} from '@/features/resumes'

export function ResumeDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data, isLoading, isError, refetch } = useResume(id)
  const [selectedVersionId, setSelectedVersionId] = useState<string | null>(null)

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
  if (isError) {
    return (
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
    )
  }
  if (!data?.resume) {
    return (
      <div className="flex-1 px-6 py-6">
        <Link to={routes.resumeLibrary()} className="text-sm text-violet-300 hover:underline">
          ← Back to resumes
        </Link>
        <div className="mt-4 text-sm text-secondary">Resume not found.</div>
      </div>
    )
  }

  const { resume, versions, attachments } = data
  const activeVersionId = selectedVersionId ?? resume.active_version_id
  const activeVersion = versions.find((v) => v.id === activeVersionId)

  return (
    <div className="flex-1 flex flex-col min-h-0 overflow-auto">
      <div className="px-6 py-6 w-full">
        {/* Breadcrumb + actions */}
        <div className="flex items-center justify-between mb-4">
          <Link
            to={routes.resumeLibrary()}
            className="inline-flex items-center gap-1.5 text-sm text-secondary hover:text-primary"
          >
            <ChevronLeft size={14} />
            Resumes
          </Link>
          <div className="flex items-center gap-1.5">
            <button
              onClick={() => toast.info('Download (mockup)')}
              className="inline-flex items-center gap-1 text-xs px-2.5 py-1 rounded bg-subtle hover:bg-hover text-secondary"
            >
              <Download size={11} />
              Download v{activeVersion?.version_number}
            </button>
            <button
              onClick={() => toast.info('Archive (mockup)')}
              className="inline-flex items-center gap-1 text-xs px-2.5 py-1 rounded bg-subtle hover:bg-hover text-secondary"
            >
              <Archive size={11} />
              Archive
            </button>
            <button
              onClick={() => navigate(routes.resumeEdit(resume.id))}
              className="inline-flex items-center gap-1 text-xs px-2.5 py-1 rounded bg-violet-600 hover:bg-violet-500 text-white"
            >
              <Edit3 size={11} />
              Edit
            </button>
          </div>
        </div>

        {/* Top row: metadata (2/3) + storage info (1/3) */}
        <ErrorBoundary>
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 mb-6">
            <div className="lg:col-span-2">
              <ResumeMetadataPanel resume={resume} />
            </div>
            <div className="rounded-xl border border-[var(--border)] bg-panel p-5">
              <div className="text-[11px] text-muted uppercase tracking-wide mb-2">Storage</div>
              <div className="text-xs text-secondary space-y-2">
                <div className="flex items-center justify-between">
                  <span className="text-secondary">Active version</span>
                  <span className="text-primary tabular-nums">
                    v{activeVersion?.version_number}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-secondary">Format</span>
                  <span className="text-primary uppercase">{activeVersion?.file_format}</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-secondary">Path</span>
                  <code className="text-[10px] text-secondary bg-black/30 px-1.5 py-0.5 rounded truncate max-w-[180px]">
                    {activeVersion?.file_path ?? '—'}
                  </code>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-secondary">Versions kept</span>
                  <span className="text-primary tabular-nums">
                    {versions.length} / {VERSION_LIMIT_PER_RESUME}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </ErrorBoundary>

        {/* Main: content (2/3) + versions (1/3) */}
        <ErrorBoundary>
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 mb-6">
            <div className="lg:col-span-2 min-w-0">
              {activeVersion ? (
                <ResumeContentPane content={activeVersion.content_text} readOnly />
              ) : (
                <div className="text-sm text-secondary">No version selected.</div>
              )}
            </div>
            <div>
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-sm font-semibold text-primary">Version history</h3>
                <span className="text-[10px] text-muted">{versions.length} total</span>
              </div>
              <ResumeVersionList
                versions={versions}
                activeId={resume.active_version_id}
                selectedId={activeVersionId}
                onSelect={setSelectedVersionId}
              />
            </div>
          </div>
        </ErrorBoundary>

        {/* Attached jobs */}
        <ErrorBoundary>
          <div>
            <h3 className="text-sm font-semibold text-primary mb-3">
              Attached jobs ({attachments.length})
            </h3>
            <AttachedJobsList resumeId={resume.id} attachments={attachments} />
          </div>
        </ErrorBoundary>
      </div>
    </div>
  )
}
