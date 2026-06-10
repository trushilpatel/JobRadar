import { Link } from 'react-router-dom'
import { Upload } from 'lucide-react'
import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import { useResumes, ResumePickerInline } from '@/features/resumes'
import { resumeQueryKeys } from '@/features/resumes'
import { routes } from '@/config/routes'

interface Props {
  jobId: string
}

// Renders inside JobDetailPanel between Tech stack and Notes.
//
// States:
//   - No resumes uploaded → "No resume found" + "Insert resume for this" CTA
//   - Resumes exist, none attached to this job → bare picker
//   - Resume already attached → picker (shows current) + state badge + "Open in editor"
//
// TODO: wire attachment to real useJobAttachment(jobId) hook once backend is ready.
export function JobResumeSection({ jobId }: Props) {
  const { data: resumes } = useResumes()
  const list = resumes ?? []
  const empty = list.length === 0

  const qc = useQueryClient()

  const handleSelect = (resumeId: string) => {
    // TODO: would POST /api/resumes/:id/attachments. Just invalidate so
    // any visible queue updates and toast a confirmation (toast lives in
    // ResumePickerInline already).
    void resumeId
    qc.invalidateQueries({ queryKey: resumeQueryKeys.applyQueue('all') })
  }

  return (
    <div>
      <h3 className="text-xs font-medium text-muted uppercase tracking-wider mb-2">Resume</h3>

      {empty ? (
        // No resumes uploaded — explicit empty state with upload CTA.
        <div className="rounded-lg border border-dashed border-[var(--border)] px-3 py-3">
          <div className="flex items-center justify-between gap-3">
            <ResumePickerInline
              jobId={jobId}
              selectedResumeId={null}
              onSelect={handleSelect}
              size="md"
            />
          </div>
          <div className="mt-2.5 text-xs text-muted">
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
        <div className="rounded-lg border border-[var(--border)] bg-subtle p-3 space-y-2.5">
          <div className="flex items-center justify-between gap-3 flex-wrap">
            <ResumePickerInline
              jobId={jobId}
              selectedResumeId={null}
              onSelect={(rid) => {
                handleSelect(rid)
                toast.info('Tip: open in editor to tailor for this job', { duration: 4000 })
              }}
              size="md"
            />
          </div>
        </div>
      )}
    </div>
  )
}
