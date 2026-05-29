import { Link } from 'react-router-dom'
import { ExternalLink, Edit3 } from 'lucide-react'
import { cn, formatRelativeTime } from '@/lib/utils/utils'
import { routes } from '@/lib/routes'
import { ResumeStateBadge } from '@/features/resumes/components/ResumeStateBadge'
import { getMockJob } from '@/features/resumes/mockData'
import type { ResumeJobAttachment } from '@/features/resumes/types'

interface Props {
  resumeId: string
  attachments: ResumeJobAttachment[]
}

export function AttachedJobsList({ resumeId, attachments }: Props) {
  if (attachments.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-white/[0.08] px-4 py-8 text-center">
        <div className="text-sm text-slate-400">Not attached to any job yet</div>
        <div className="text-xs text-slate-500 mt-1">
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
              'rounded-lg border border-white/[0.05] bg-[#141420] px-3 py-2.5',
              'flex items-center justify-between gap-3'
            )}
          >
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 mb-0.5">
                <ResumeStateBadge state={a.state} />
                <span className="text-[10px] text-slate-500">
                  Attached {formatRelativeTime(a.attached_at)}
                </span>
              </div>
              <div className="text-sm text-white truncate">{job.title}</div>
              <div className="text-xs text-slate-400 truncate">
                {job.company} · {job.location}
              </div>
            </div>
            <div className="flex items-center gap-1 shrink-0">
              {a.state !== 'submitted' && (
                <Link
                  to={routes.resumeEdit(resumeId, { jobId: job.id })}
                  className="text-xs px-2 py-1 rounded bg-white/[0.04] hover:bg-white/[0.08] text-slate-300 inline-flex items-center gap-1"
                >
                  <Edit3 size={11} />
                  Edit
                </Link>
              )}
              <a
                href={job.url}
                target="_blank"
                rel="noreferrer"
                className="text-slate-400 hover:text-white p-1"
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
