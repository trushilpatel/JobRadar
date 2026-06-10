// Resume feature types — mirrors the planned backend schema. Backend slice
// hasn't been built yet; these types are the source of truth for the mockup
// and will be the shape returned by `/admin/resumes/*` once wired.

export type ResumeAxis = 'role' | 'seniority' | 'industry' | 'stack' | 'geo'

export interface ResumeAxisTag {
  axis: ResumeAxis
  value: string
}

export type ResumeFileFormat = 'pdf' | 'docx' | 'text'

export type ResumeAttachmentState = 'draft' | 'final' | 'submitted'

// `user_job_state.status` overlay surfaced in the apply queue.
export type JobLifecycleStatus = 'saved' | 'applied' | 'interviewing' | 'offer'

// `user_resumes` row
export interface Resume {
  id: string
  user_id: string
  name: string
  label: string
  tags: ResumeAxisTag[]
  active_version_id: string
  version_count: number
  attached_job_count: number
  submitted_count: number
  created_at: string
  updated_at: string
  archived_at: string | null
}

// `resume_versions` row — immutable snapshot.
// Storage path scheme: resumes/{user_id}/{resume_id}/v{version_number}.{ext}
export interface ResumeVersion {
  id: string
  resume_id: string
  version_number: number
  content_text: string
  file_path: string | null
  file_format: ResumeFileFormat
  size_bytes: number
  change_summary: string | null
  created_at: string
  is_archived: boolean
}

// `resume_job_attachments` row
export interface ResumeJobAttachment {
  id: string
  resume_id: string
  version_id: string
  job_id: string
  state: ResumeAttachmentState
  notes: string | null
  attached_at: string
  finalized_at: string | null
  submitted_at: string | null
}

// View model used by ApplyQueuePage — joins job snapshot + attachment + resume meta.
// `attachment` is null when the job is saved but no resume is attached yet.
// `resume` is null in the same case.
export interface ApplyQueueItem {
  attachment: ResumeJobAttachment | null
  job: {
    id: string
    title: string
    company: string
    location: string
    url: string
    salary?: string
    saved_at: string
    // From user_job_state — drives whether row appears in Interviewing tab.
    lifecycle_status: JobLifecycleStatus
    // Analyzer-populated fields — undefined until the job has been processed.
    responsibilities?: string[]
    requirements?: string[]
    role_brief?: string
    tech_stack?: string[]
  }
  resume: {
    id: string
    name: string
    version_number: number
  } | null
}

export const RESUME_LIMIT_PER_USER = 50
export const VERSION_LIMIT_PER_RESUME = 20
export const FILE_MAX_BYTES = 2 * 1024 * 1024
