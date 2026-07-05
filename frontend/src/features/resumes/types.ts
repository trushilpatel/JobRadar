// Resume feature types — mirrors the flat `resumes` table (migration 086).
// No versioning/tags/attachment fields: those belong to the larger,
// not-yet-built feature in docs/design/resume-feature-spec.md.

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
  // Keyword extraction (issue #264) — always an array, empty when extraction
  // hasn't run yet or produced no keywords. Never null/undefined at this layer;
  // the API function normalizes.
  keywords: string[]
}

export const RESUME_LIMIT_PER_USER = 50
export const RESUME_MAX_FILE_BYTES = 2 * 1024 * 1024
export const RESUME_ALLOWED_EXTENSIONS = ['.pdf', '.docx'] as const
// Cap on how many keyword badges ResumeCard renders before collapsing the
// rest into a "+N more" affordance — keeps card height bounded.
export const RESUME_KEYWORDS_VISIBLE_LIMIT = 6
