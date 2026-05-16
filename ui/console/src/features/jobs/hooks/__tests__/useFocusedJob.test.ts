import { describe, it, expect } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useFocusedJob } from '@/features/jobs/hooks/useFocusedJob'
import { JOB_STATUS } from '@/constants/jobStatus'
import type { Job } from '@/types'

function makeJob(id: string): Job {
  return {
    id,
    job_post_id: `p-${id}`,
    company_id: 'c1',
    title: `Title ${id}`,
    location: 'Remote',
    remote: true,
    url: `https://x/${id}`,
    department: null,
    employ_type: null,
    experience_years: null,
    tech_stack: null,
    salary_range: null,
    llm_summary: null,
    sponsorship: false,
    requires_citizen: false,
    security_clearance: false,
    role_brief: null,
    responsibilities: null,
    status: JOB_STATUS.NEW,
    saved: false,
    notes: null,
    user_state: {
      status: JOB_STATUS.NEW,
      notes: null,
      saved_at: null,
      applied_at: null,
      dismissed_at: null,
      dismissal_reason: null,
      dismissal_note: null,
    },
    added_at: '2024-01-01T00:00:00Z',
    company: {
      id: 'c1',
      name: 'Acme',
      url: 'https://acme.com',
      adapter_type: 'greenhouse',
      active: true,
      created_at: '2024-01-01T00:00:00Z',
      last_scan_at: null,
      last_scan_status: null,
    },
  }
}

const JOBS = [makeJob('a'), makeJob('b'), makeJob('c')]

describe('useFocusedJob', () => {
  it('starts focused at index 0 with the first job', () => {
    const { result } = renderHook(() => useFocusedJob(JOBS))
    expect(result.current.focusedIndex).toBe(0)
    expect(result.current.focusedJob?.id).toBe('a')
  })

  it('focusedJob is null when the list is empty', () => {
    const { result } = renderHook(() => useFocusedJob([]))
    expect(result.current.focusedJob).toBeNull()
  })

  it('focusNext advances and clamps at the last index', () => {
    const { result } = renderHook(() => useFocusedJob(JOBS))
    act(() => result.current.focusNext())
    expect(result.current.focusedIndex).toBe(1)
    act(() => result.current.focusNext())
    expect(result.current.focusedIndex).toBe(2)
    act(() => result.current.focusNext())
    expect(result.current.focusedIndex).toBe(2)
  })

  it('focusPrev decrements and clamps at 0', () => {
    const { result } = renderHook(() => useFocusedJob(JOBS))
    act(() => {
      result.current.focusNext()
      result.current.focusNext()
    })
    expect(result.current.focusedIndex).toBe(2)
    act(() => result.current.focusPrev())
    expect(result.current.focusedIndex).toBe(1)
    act(() => result.current.focusPrev())
    expect(result.current.focusedIndex).toBe(0)
    act(() => result.current.focusPrev())
    expect(result.current.focusedIndex).toBe(0)
  })

  it('focusFirst and focusLast jump to the ends', () => {
    const { result } = renderHook(() => useFocusedJob(JOBS))
    act(() => result.current.focusLast())
    expect(result.current.focusedIndex).toBe(2)
    act(() => result.current.focusFirst())
    expect(result.current.focusedIndex).toBe(0)
  })

  it('focusLast clamps to 0 when the list is empty', () => {
    const { result } = renderHook(() => useFocusedJob([]))
    act(() => result.current.focusLast())
    expect(result.current.focusedIndex).toBe(0)
  })

  it('resets focus to 0 when the jobs reference changes', () => {
    const { result, rerender } = renderHook(({ jobs }) => useFocusedJob(jobs), {
      initialProps: { jobs: JOBS },
    })
    act(() => {
      result.current.focusLast()
    })
    expect(result.current.focusedIndex).toBe(2)

    rerender({ jobs: [makeJob('x'), makeJob('y')] })
    expect(result.current.focusedIndex).toBe(0)
    expect(result.current.focusedJob?.id).toBe('x')
  })

  it('setFocusedIndex allows explicit positioning', () => {
    const { result } = renderHook(() => useFocusedJob(JOBS))
    act(() => result.current.setFocusedIndex(2))
    expect(result.current.focusedJob?.id).toBe('c')
  })
})
