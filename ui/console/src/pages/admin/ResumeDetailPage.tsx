import { useState } from 'react'
import { Link, useParams, useNavigate } from 'react-router-dom'
import { ChevronLeft, Edit3, Archive, Download } from 'lucide-react'
import { toast } from 'sonner'
import { routes } from '@/config/routes'
import {
  useResume,
  ResumeMetadataPanel,
  ResumeVersionList,
  ResumeContentPane,
  AttachedJobsList,
} from '@/features/resumes'

export function ResumeDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data, isLoading } = useResume(id)
  const [selectedVersionId, setSelectedVersionId] = useState<string | null>(null)

  if (isLoading) {
    return <div className="flex-1 px-6 py-6 text-sm text-slate-400">Loading…</div>
  }
  if (!data?.resume) {
    return (
      <div className="flex-1 px-6 py-6">
        <Link to={routes.resumeLibrary()} className="text-sm text-violet-300 hover:underline">
          ← Back to resumes
        </Link>
        <div className="mt-4 text-sm text-slate-400">Resume not found.</div>
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
            className="inline-flex items-center gap-1.5 text-sm text-slate-400 hover:text-white"
          >
            <ChevronLeft size={14} />
            Resumes
          </Link>
          <div className="flex items-center gap-1.5">
            <button
              onClick={() => toast.info('Download (mockup)')}
              className="inline-flex items-center gap-1 text-xs px-2.5 py-1 rounded bg-white/[0.04] hover:bg-white/[0.08] text-slate-300"
            >
              <Download size={11} />
              Download v{activeVersion?.version_number}
            </button>
            <button
              onClick={() => toast.info('Archive (mockup)')}
              className="inline-flex items-center gap-1 text-xs px-2.5 py-1 rounded bg-white/[0.04] hover:bg-white/[0.08] text-slate-300"
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
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 mb-6">
          <div className="lg:col-span-2">
            <ResumeMetadataPanel resume={resume} />
          </div>
          <div className="rounded-xl border border-white/[0.06] bg-[#141420] p-5">
            <div className="text-[11px] text-slate-500 uppercase tracking-wide mb-2">Storage</div>
            <div className="text-xs text-slate-300 space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Active version</span>
                <span className="text-white tabular-nums">v{activeVersion?.version_number}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Format</span>
                <span className="text-white uppercase">{activeVersion?.file_format}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Path</span>
                <code className="text-[10px] text-slate-300 bg-black/30 px-1.5 py-0.5 rounded truncate max-w-[180px]">
                  {activeVersion?.file_path ?? '—'}
                </code>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-400">Versions kept</span>
                <span className="text-white tabular-nums">{versions.length} / 20</span>
              </div>
            </div>
          </div>
        </div>

        {/* Main: content (2/3) + versions (1/3) */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 mb-6">
          <div className="lg:col-span-2 min-w-0">
            {activeVersion ? (
              <ResumeContentPane content={activeVersion.content_text} readOnly />
            ) : (
              <div className="text-sm text-slate-400">No version selected.</div>
            )}
          </div>
          <div>
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-semibold text-white">Version history</h3>
              <span className="text-[10px] text-slate-500">{versions.length} total</span>
            </div>
            <ResumeVersionList
              versions={versions}
              activeId={resume.active_version_id}
              selectedId={activeVersionId}
              onSelect={setSelectedVersionId}
            />
          </div>
        </div>

        {/* Attached jobs */}
        <div>
          <h3 className="text-sm font-semibold text-white mb-3">
            Attached jobs ({attachments.length})
          </h3>
          <AttachedJobsList resumeId={resume.id} attachments={attachments} />
        </div>
      </div>
    </div>
  )
}
