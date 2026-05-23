import { Link } from 'react-router-dom'
import { Upload, ArrowUpRight } from 'lucide-react'
import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import { useResumes } from '@/features/resumes/hooks/useResumes'
import { getMockAttachmentForJob } from '@/features/resumes/mockData'
import { ResumePickerInline } from '@/features/resumes/components/ResumePickerInline'
import { ResumeStateBadge } from '@/features/resumes/components/ResumeStateBadge'
import { resumeQueryKeys } from '@/features/resumes/queryKeys'
import { routes } from '@/lib/routes'

interface Props {
  jobId: string
}

// Renders inside JobDetailPanel between Tech stack and Notes.
//
// States:
//   - No resumes uploaded → "No resume found" + "Insert resume for this" CTA
//   - Resumes exist, none attached to this job → bare picker
//   - Resume already attached → picker (shows current) + state badge + "Open in editor"
export function JobResumeSection({ jobId }: Props) {
  const { data: resumes } = useResumes()
  const list = resumes ?? []
  const empty = list.length === 0

  const qc = useQueryClient()
  // Mockup: read directly from the static map. Real impl will read from
  // user_job_state via a `useJobAttachment(jobId)` hook.
  const attachment = getMockAttachmentForJob(jobId)
  const attachedResume = attachment ? list.find((r) => r.id === attachment.resume_id) : null

  const handleSelect = (resumeId: string) => {
    // Mockup — would POST /api/resumes/:id/attachments. Just invalidate so
    // any visible queue updates and toast a confirmation (toast lives in
    // ResumePickerInline already).
    void resumeId
    qc.invalidateQueries({ queryKey: resumeQueryKeys.applyQueue('all') })
  }

  return (
    <div>
      <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Resume</h3>

      {empty ? (
        // No resumes uploaded — explicit empty state with upload CTA.
        <div className="rounded-lg border border-dashed border-white/[0.08] px-3 py-3">
          <div className="flex items-center justify-between gap-3">
            <ResumePickerInline
              jobId={jobId}
              selectedResumeId={null}
              onSelect={handleSelect}
              size="md"
            />
          </div>
          <div className="mt-2.5 text-xs text-slate-400">
            No resume found.{' '}
            <Link
              to={routes.resumeLibrary()}
              className="text-violet-300 hover:text-violet-200 inline-flex items-center gap-0.5"
            >
              Insert a resume for this
              <Upload size={11} className="ml-0.5" />
            </Link>
          </div>
        </div>
      ) : (
        <div className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-3 space-y-2.5">
          <div className="flex items-center justify-between gap-3 flex-wrap">
            <ResumePickerInline
              jobId={jobId}
              selectedResumeId={attachedResume?.id ?? null}
              onSelect={(rid) => {
                handleSelect(rid)
                toast.info('Tip: open in editor to tailor for this job', { duration: 4000 })
              }}
              size="md"
            />
            {attachment && (
              <div className="flex items-center gap-2">
                <ResumeStateBadge state={attachment.state} />
              </div>
            )}
          </div>

          {attachedResume && attachment && (
            <div className="flex items-center justify-between gap-2 text-xs">
              <span className="text-slate-500">
                v{list.find((r) => r.id === attachment.resume_id)?.version_count ?? '?'} · attached
                for this job
              </span>
              <Link
                to={routes.resumeEdit(attachedResume.id, { jobId })}
                className="text-violet-300 hover:text-violet-200 inline-flex items-center gap-0.5"
              >
                Open in editor
                <ArrowUpRight size={11} />
              </Link>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
