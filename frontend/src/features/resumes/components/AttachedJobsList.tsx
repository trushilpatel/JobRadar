import { Link } from 'react-router-dom'
import { ExternalLink, Edit3 } from 'lucide-react'
import { cn, formatRelativeTime } from '@/lib/utils/utils'
import { routes } from '@/config/routes'
import { ResumeStateBadge } from './ResumeStateBadge'
import { getMockJob } from '../mockData'
import type { ResumeJobAttachment } from '../types'

interface Props {
  resumeId: string
  attachments: ResumeJobAttachment[]
}

export function AttachedJobsList({ resumeId, attachments }: Props) {
  if (attachments.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-[var(--border-strong)] px-4 py-8 text-center">
        <div className="text-sm text-muted">Not attached to any job yet</div>
        <div className="text-xs text-muted mt-1">
          Attach this resume from the Apply Queue or any saved job posting.
        </div>
      </div>
    )
  }
  return (
    <div className="space-y-2">
      {attachments.map((a) => {
        const job = getMockJob(a.job_id)
        if (!job) return null
        return (
          <div
            key={a.id}
            className={cn(
              'rounded-lg border border-[var(--border)] bg-panel px-3 py-2.5',
              'flex items-center justify-between gap-3'
            )}
          >
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 mb-0.5">
                <ResumeStateBadge state={a.state} />
                <span className="text-[10px] text-muted">
                  Attached {formatRelativeTime(a.attached_at)}
                </span>
              </div>
              <div className="text-sm text-primary truncate">{job.title}</div>
              <div className="text-xs text-muted truncate">
                {job.company} · {job.location}
              </div>
            </div>
            <div className="flex items-center gap-1 shrink-0">
              {a.state !== 'submitted' && (
                <Link
                  to={routes.resumeEdit(resumeId, { jobId: job.id })}
                  className="text-xs px-2 py-1 rounded bg-subtle hover:bg-hover text-secondary inline-flex items-center gap-1"
                >
                  <Edit3 size={11} />
                  Edit
                </Link>
              )}
              <a
                href={job.url}
                target="_blank"
                rel="noreferrer"
                className="text-muted hover:text-primary p-1"
                title="Open job posting"
              >
                <ExternalLink size={12} />
              </a>
            </div>
          </div>
        )
      })}
    </div>
  )
}
