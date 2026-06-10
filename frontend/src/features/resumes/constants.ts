// Shared constants for the resumes feature slice.

// Base axis tone classes (background + text only — no ring). Used by compact
// components such as ResumeCard where a ring would be visually heavy.
export const AXIS_TONE: Record<string, string> = {
  role: 'bg-violet-500/10 text-violet-300',
  seniority: 'bg-sky-500/10 text-sky-300',
  industry: 'bg-rose-500/10 text-rose-300',
  stack: 'bg-emerald-500/10 text-emerald-300',
  geo: 'bg-amber-500/10 text-amber-300',
}

// Axis tone classes including a ring — used by detail-level components such as
// ResumeMetadataPanel where additional visual depth is appropriate.
export const AXIS_TONE_WITH_RING: Record<string, string> = {
  role: 'bg-violet-500/10 text-violet-300 ring-violet-500/20',
  seniority: 'bg-sky-500/10 text-sky-300 ring-sky-500/20',
  industry: 'bg-rose-500/10 text-rose-300 ring-rose-500/20',
  stack: 'bg-emerald-500/10 text-emerald-300 ring-emerald-500/20',
  geo: 'bg-amber-500/10 text-amber-300 ring-amber-500/20',
}
