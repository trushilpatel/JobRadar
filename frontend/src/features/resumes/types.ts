// Resume feature types — mirrors the flat `resumes` table (migration 086,
// extracted_text added in 090). No versioning/tags/attachment fields: those
// belong to the larger, not-yet-built feature in
// docs/design/resume-feature-spec.md.

export interface Resume {
  id: string
  user_id: string
  label: string
  original_filename: string
  content_type:
    | 'application/pdf'
    | 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'
  size_bytes: number
  created_at: string
  // Plain text extracted at upload (no LLM) — internal matching input only,
  // never rendered directly. See
  // docs/superpowers/specs/2026-07-05-resume-job-keyword-match-design.md.
  extracted_text: string
}

export const RESUME_LIMIT_PER_USER = 50
export const RESUME_MAX_FILE_BYTES = 2 * 1024 * 1024
export const RESUME_ALLOWED_EXTENSIONS = ['.pdf', '.docx'] as const
