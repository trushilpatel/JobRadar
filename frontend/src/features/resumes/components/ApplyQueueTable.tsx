import { Link, useNavigate } from 'react-router-dom'
import { ExternalLink, Edit3, Send, Paperclip } from 'lucide-react'
import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import { cn, formatRelativeTime } from '@/lib/utils/utils'
import { routes } from '@/config/routes'
import { resumeQueryKeys } from '../queryKeys'
import { ResumeStateBadge } from './ResumeStateBadge'
import { ResumePickerInline } from './ResumePickerInline'
import type { ApplyQueueItem } from '../types'

interface Props {
  items: ApplyQueueItem[]
  isLoading?: boolean
  emptyMessage?: string
}

const LIFECYCLE_TONE: Record<string, string> = {
  saved: 'text-muted',
  applied: 'text-sky-300',
  interviewing: 'text-amber-300',
  offer: 'text-emerald-300',
}

export function ApplyQueueTable({ items, isLoading, emptyMessage }: Props) {
  const navigate = useNavigate()
  const qc = useQueryClient()

  if (isLoading) {
    return (
      <div className="space-y-2 animate-pulse pt-4">
        {Array.from({ length: 5 }).map((_, i) => (
          <div key={i} className="h-14 rounded-lg bg-slate-800/60" />
        ))}
      </div>
    )
  }
  if (items.length === 0) {
    return (
      <div className="text-sm text-muted italic py-12 text-center border border-dashed border-[var(--border)] rounded-lg">
        {emptyMessage ?? 'No jobs in this state.'}
      </div>
    )
  }

  const handleRowClick = (jobId: string) => {
    navigate(routes.jobOpen(jobId))
  }

  const handleAttach = (jobId: string, resumeId: string) => {
    // Mockup: just invalidate the queue so it refetches. Real impl will
    // POST /api/resumes/:resumeId/attachments {job_id}.
    void resumeId
    void jobId
    qc.invalidateQueries({ queryKey: resumeQueryKeys.applyQueue('all') })
  }

  const handleSubmit = (jobId: string, resumeName: string, version: number) => {
    toast.success(`Marked submitted: ${resumeName} v${version}`)
    qc.invalidateQueries({ queryKey: ['resumes'] })
    qc.invalidateQueries({ queryKey: ['apply-queue'] })
    void jobId
  }

  return (
    <div className="rounded-xl border border-[var(--border)] overflow-hidden">
      <table className="w-full text-sm">
        <thead className="bg-subtle border-b border-[var(--border)]">
          <tr className="text-left text-[11px] uppercase tracking-wide text-muted">
            <th className="px-4 py-2 font-medium">Job</th>
            <th className="px-3 py-2 font-medium w-[100px]">Saved</th>
            <th className="px-3 py-2 font-medium w-[260px]">Resume</th>
            <th className="px-3 py-2 font-medium w-[120px]">State</th>
            <th className="px-3 py-2 font-medium w-[140px] text-right">Actions</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <ApplyQueueRow
              key={item.job.id + (item.attachment?.id ?? '-')}
              item={item}
              onRowClick={handleRowClick}
              onAttach={handleAttach}
              onSubmit={handleSubmit}
            />
          ))}
        </tbody>
      </table>
    </div>
  )
}

interface RowProps {
  item: ApplyQueueItem
  onRowClick: (jobId: string) => void
  onAttach: (jobId: string, resumeId: string) => void
  onSubmit: (jobId: string, resumeName: string, version: number) => void
}

function ApplyQueueRow({ item, onRowClick, onAttach, onSubmit }: RowProps) {
  const { job, resume, attachment } = item
  const lifecycle = job.lifecycle_status

  return (
    <tr
      className="border-b border-[var(--border)] last:border-0 hover:bg-hover cursor-pointer transition-colors"
      onClick={(e) => {
        // Don't navigate when clicks land on interactive children
        const t = e.target as HTMLElement
        if (t.closest('[data-no-row-click]')) return
        onRowClick(job.id)
      }}
    >
      {/* Job */}
      <td className="px-4 py-3 min-w-0">
        <div className="flex items-center gap-2 min-w-0">
          {attachment && <Paperclip size={11} className="text-violet-400 shrink-0" />}
          <div className="min-w-0">
            <div className="text-primary truncate">{job.title}</div>
            <div className="text-xs text-muted truncate">
              {job.company} · {job.location}
              {job.salary && <span className="text-muted"> · {job.salary}</span>}
            </div>
          </div>
        </div>
      </td>

      {/* Saved */}
      <td className="px-3 py-3 align-middle">
        <div className="text-xs text-muted">{formatRelativeTime(job.saved_at)}</div>
        {lifecycle !== 'saved' && (
          <div
            className={cn('text-[10px] uppercase tracking-wide mt-0.5', LIFECYCLE_TONE[lifecycle])}
          >
            {lifecycle}
          </div>
        )}
      </td>

      {/* Resume — inline dropdown */}
      <td className="px-3 py-3 align-middle" data-no-row-click>
        <div className="flex items-center gap-2">
          <ResumePickerInline
            jobId={job.id}
            selectedResumeId={resume?.id ?? null}
            onSelect={(resumeId) => onAttach(job.id, resumeId)}
          />
          {resume && (
            <span className="text-[10px] text-muted tabular-nums shrink-0">
              v{resume.version_number}
            </span>
          )}
        </div>
      </td>

      {/* State */}
      <td className="px-3 py-3 align-middle">
        {attachment ? (
          <ResumeStateBadge state={attachment.state} />
        ) : (
          <span className="text-[11px] text-muted italic">Not attached</span>
        )}
      </td>

      {/* Actions */}
      <td className="px-3 py-3 align-middle" data-no-row-click>
        <div className="flex items-center justify-end gap-1">
          {resume && attachment && attachment.state !== 'submitted' && (
            <Link
              to={routes.resumeEdit(resume.id, { jobId: job.id })}
              className="inline-flex items-center gap-1 text-xs px-2 py-1 rounded bg-subtle hover:bg-hover text-secondary"
              title="Edit resume for this job"
            >
              <Edit3 size={11} />
              Edit
            </Link>
          )}
          {attachment?.state === 'final' && resume && (
            <button
              onClick={() => onSubmit(job.id, resume.name, resume.version_number)}
              className="inline-flex items-center gap-1 text-xs px-2 py-1 rounded bg-violet-600 hover:bg-violet-500 text-white"
              title="Mark submitted"
            >
              <Send size={11} />
              Submit
            </button>
          )}
          <a
            href={job.url}
            target="_blank"
            rel="noreferrer"
            className="text-muted hover:text-primary p-1"
            title="Open job posting in new tab"
          >
            <ExternalLink size={12} />
          </a>
        </div>
      </td>
    </tr>
  )
}
