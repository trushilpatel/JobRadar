import { describe, it, expect } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useApplyQueueResumeMatch } from '../useApplyQueueResumeMatch'
import type { ApplyQueueItem } from '@/features/apply-queue'
import type { Resume } from '@/features/resumes'

function makeItem(overrides: Partial<ApplyQueueItem>): ApplyQueueItem {
  return {
    job_id: 'job-1',
    job_title: 'Engineer',
    company: 'Acme',
    job_url: 'https://x',
    status: 'saved',
    resume_id: null,
    resume_label: null,
    job_keywords: [],
    location: null,
    remote: false,
    salary_range: null,
    added_at: '2026-07-01T00:00:00Z',
    ...overrides,
  }
}

function makeResume(overrides: Partial<Resume>): Resume {
  return {
    id: 'r1',
    user_id: 'u1',
    label: 'My resume',
    original_filename: 'r.pdf',
    content_type: 'application/pdf',
    size_bytes: 1024,
    created_at: '2026-01-01T00:00:00Z',
    extracted_text: '',
    ...overrides,
  }
}

describe('useApplyQueueResumeMatch', () => {
  it('auto-picks the best-matching resume when none is attached', () => {
    const items = [
      makeItem({ job_id: 'job-1', job_keywords: ['Go', 'PostgreSQL'], resume_id: null }),
    ]
    const resumes = [makeResume({ id: 'r1', extracted_text: 'Go and PostgreSQL experience.' })]
    const { result } = renderHook(() => useApplyQueueResumeMatch(items, resumes))
    expect(result.current.getEffectiveResume('job-1')).toEqual({ id: 'r1', label: 'My resume' })
    expect(result.current.getMatchSummary('job-1')).toBe('2/2 matched')
  })

  it('prefers the user-attached resume over the auto-pick', () => {
    const items = [
      makeItem({
        job_id: 'job-1',
        job_keywords: ['Go', 'PostgreSQL'],
        resume_id: 'r2',
        resume_label: 'Manually chosen',
      }),
    ]
    const resumes = [
      makeResume({ id: 'r1', label: 'Best match', extracted_text: 'Go and PostgreSQL.' }),
      makeResume({
        id: 'r2',
        label: 'Manually chosen',
        extracted_text: 'Totally unrelated content.',
      }),
    ]
    const { result } = renderHook(() => useApplyQueueResumeMatch(items, resumes))
    expect(result.current.getEffectiveResume('job-1')).toEqual({
      id: 'r2',
      label: 'Manually chosen',
    })
    expect(result.current.getMatchSummary('job-1')).toBe('0/2 matched')
  })

  it('returns null for both when the job has no keywords', () => {
    const items = [makeItem({ job_id: 'job-1', job_keywords: [], resume_id: null })]
    const resumes = [makeResume({ id: 'r1' })]
    const { result } = renderHook(() => useApplyQueueResumeMatch(items, resumes))
    expect(result.current.getEffectiveResume('job-1')).toBeNull()
    expect(result.current.getMatchSummary('job-1')).toBeNull()
  })

  it('returns null for an unknown jobId', () => {
    const { result } = renderHook(() => useApplyQueueResumeMatch([], []))
    expect(result.current.getEffectiveResume('nope')).toBeNull()
    expect(result.current.getMatchSummary('nope')).toBeNull()
  })
})
