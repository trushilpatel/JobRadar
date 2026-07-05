// Pure, LLM-free matching between a job's canonical keywords and a resume's
// extracted plain text — see
// docs/superpowers/specs/2026-07-05-resume-job-keyword-match-design.md.
// Lives in lib/ (not a feature) because it is used by the page layer that
// composes features/apply-queue and features/resumes, which may not import
// each other directly.

// escapeRegExp escapes regex-special characters in a keyword so it can be
// safely embedded in a RegExp source string — required for keywords like
// "C++", "C#", and "Node.js" that contain characters with regex meaning.
function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// keywordRegExp builds a case-insensitive matcher requiring the keyword not
// be directly adjacent to an alphanumeric character on either side —
// deliberately NOT a plain \b: \b only fires at a transition between a word
// char and a non-word char, so a keyword ending in a symbol (e.g. "C++")
// would fail to match when followed by whitespace, since both the trailing
// "+" and the following space are non-word characters (no transition).
// Checking alphanumeric adjacency directly handles "C++", "C#", "Node.js"
// correctly while still rejecting "Go" as a substring match inside "Google".
function keywordRegExp(keyword: string): RegExp {
  return new RegExp(`(?<![a-zA-Z0-9])${escapeRegExp(keyword)}(?![a-zA-Z0-9])`, 'i')
}

export interface MatchResult {
  matched: number
  total: number
}

// matchResume counts how many of jobKeywords appear as whole-word,
// case-insensitive matches in resumeText.
export function matchResume(jobKeywords: string[], resumeText: string): MatchResult {
  const total = jobKeywords.length
  if (total === 0) return { matched: 0, total: 0 }

  let matched = 0
  for (const keyword of jobKeywords) {
    if (keywordRegExp(keyword).test(resumeText)) matched++
  }
  return { matched, total }
}

export interface ResumeForMatch {
  id: string
  label: string
  extractedText: string
}

export interface ResumeMatchPick {
  id: string
  label: string
}

// pickBestResume returns the resume with the highest matchResume score among
// resumes, or null when there are no job keywords, no resumes, or every
// resume scores zero (no recommendation is better than an arbitrary one).
// Ties keep the first-encountered resume — callers pass resumes pre-sorted
// oldest-first so this deterministically prefers the earliest upload,
// mirroring the backend's prior tie-break semantics.
export function pickBestResume(
  jobKeywords: string[],
  resumes: ResumeForMatch[]
): ResumeMatchPick | null {
  if (jobKeywords.length === 0 || resumes.length === 0) return null

  let bestScore = 0
  let best: ResumeMatchPick | null = null
  for (const resume of resumes) {
    const { matched } = matchResume(jobKeywords, resume.extractedText)
    if (matched > bestScore) {
      bestScore = matched
      best = { id: resume.id, label: resume.label }
    }
  }
  return best
}
