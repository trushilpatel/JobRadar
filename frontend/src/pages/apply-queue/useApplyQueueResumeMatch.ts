import { useMemo } from 'react'
import {
  matchResume,
  matchResumeDetailed,
  pickBestResume,
  type ResumeForMatch,
} from '@/lib/utils/resumeJobMatch'
import type { ApplyQueueItem, EffectiveResume, ResumeMatchOption } from '@/features/apply-queue'
import type { Resume } from '@/features/resumes'

// useApplyQueueResumeMatch computes, per apply-queue row, which resume is
// "effectively" selected (the user's explicit attachment, falling back to
// the best client-side auto-pick) and a one-line match summary against it.
// Lives under pages/ (not features/) because it composes data from both
// features/apply-queue and features/resumes, which may not import each
// other — see features/CLAUDE.md.
//
// The auto-pick is purely a display computation: it is never written back
// to the backend as a real attachment. If the user attaches a different
// resume (or re-confirms the same one), that's a real write via the existing
// setApplyQueueResume mutation, unrelated to this hook.
export function useApplyQueueResumeMatch(items: ApplyQueueItem[], resumes: Resume[]) {
  const resumesForMatch: ResumeForMatch[] = useMemo(
    () => resumes.map((r) => ({ id: r.id, label: r.label, extractedText: r.extracted_text })),
    [resumes]
  )

  const autoPickByJobId = useMemo(() => {
    const map = new Map<string, EffectiveResume>()
    for (const item of items) {
      const best = pickBestResume(item.job_keywords, resumesForMatch)
      if (best) map.set(item.job_id, best)
    }
    return map
  }, [items, resumesForMatch])

  const itemByJobId = useMemo(() => {
    const map = new Map<string, ApplyQueueItem>()
    for (const item of items) map.set(item.job_id, item)
    return map
  }, [items])

  function getEffectiveResume(jobId: string): EffectiveResume | null {
    const item = itemByJobId.get(jobId)
    if (!item) return null
    if (item.resume_id) return { id: item.resume_id, label: item.resume_label ?? 'Unknown resume' }
    return autoPickByJobId.get(jobId) ?? null
  }

  function getMatchSummary(jobId: string): string | null {
    const item = itemByJobId.get(jobId)
    if (!item) return null
    const effective = getEffectiveResume(jobId)
    if (!effective) return null
    const resume = resumesForMatch.find((r) => r.id === effective.id)
    if (!resume) return null
    const { matched, total } = matchResume(item.job_keywords, resume.extractedText)
    if (total === 0) return null
    return `${matched}/${total} matched`
  }

  // Resume-picker dialog data: every resume's match detail against this job,
  // sorted best-match-first. Array.prototype.sort is stable, so ties keep
  // resumesForMatch's original (upload) order — same tie-break semantics as
  // pickBestResume.
  function getResumeMatchOptions(jobId: string): ResumeMatchOption[] {
    const item = itemByJobId.get(jobId)
    if (!item) return []
    return resumesForMatch
      .map((r) => {
        const { matchedKeywords, missingKeywords } = matchResumeDetailed(
          item.job_keywords,
          r.extractedText
        )
        return {
          id: r.id,
          label: r.label,
          matched: matchedKeywords.length,
          total: item.job_keywords.length,
          matchedKeywords,
          missingKeywords,
          hasExtractedText: r.extractedText.length > 0,
        }
      })
      .sort((a, b) => b.matched - a.matched)
  }

  return { getEffectiveResume, getMatchSummary, getResumeMatchOptions }
}
