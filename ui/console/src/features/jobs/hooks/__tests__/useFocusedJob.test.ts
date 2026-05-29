import { describe, it, expect } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useFocusedJob } from '@/features/jobs/hooks/useFocusedJob'
import { JOB_STATUS } from '@/config/constants/jobStatus'
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
  it('starts focused on the first job', () => {
    const { result } = renderHook(() => useFocusedJob(JOBS))
    expect(result.current.focusedIndex).toBe(0)
    expect(result.current.focusedJobId).toBe('a')
    expect(result.current.focusedJob?.id).toBe('a')
  })

  it('returns null/−1 when the list is empty', () => {
    const { result } = renderHook(() => useFocusedJob([]))
    expect(result.current.focusedJob).toBeNull()
    expect(result.current.focusedJobId).toBeNull()
    expect(result.current.focusedIndex).toBe(-1)
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

  it('navigation is a no-op when the list is empty', () => {
    const { result } = renderHook(() => useFocusedJob([]))
    act(() => result.current.focusLast())
    expect(result.current.focusedIndex).toBe(-1)
    act(() => result.current.focusNext())
    expect(result.current.focusedIndex).toBe(-1)
  })

  it('setFocusedIndex allows explicit positioning', () => {
    const { result } = renderHook(() => useFocusedJob(JOBS))
    act(() => result.current.setFocusedIndex(2))
    expect(result.current.focusedJob?.id).toBe('c')
  })

  it('setFocusedJobId moves focus to the named job', () => {
    const { result } = renderHook(() => useFocusedJob(JOBS))
    act(() => result.current.setFocusedJobId('b'))
    expect(result.current.focusedIndex).toBe(1)
    expect(result.current.focusedJobId).toBe('b')
  })

  it('setFocusedJobId with an unknown id clears focus', () => {
    const { result } = renderHook(() => useFocusedJob(JOBS))
    act(() => result.current.setFocusedJobId('ghost'))
    expect(result.current.focusedJobId).toBeNull()
  })

  it('PRESERVES focus across a TanStack refetch (same ids, new array reference)', () => {
    const { result, rerender } = renderHook(({ jobs }) => useFocusedJob(jobs), {
      initialProps: { jobs: JOBS },
    })
    act(() => result.current.focusLast())
    expect(result.current.focusedJobId).toBe('c')

    // Simulate refetch: brand-new array, same ids.
    const refetched = [makeJob('a'), makeJob('b'), makeJob('c')]
    rerender({ jobs: refetched })

    expect(result.current.focusedJobId).toBe('c')
    expect(result.current.focusedIndex).toBe(2)
  })

  it('CLAMPS focus to the same index when the focused job is removed', () => {
    const { result, rerender } = renderHook(({ jobs }) => useFocusedJob(jobs), {
      initialProps: { jobs: JOBS },
    })
    act(() => result.current.setFocusedJobId('b'))
    expect(result.current.focusedIndex).toBe(1)

    // 'b' is gone; cursor should land on whatever now sits at index 1.
    rerender({ jobs: [makeJob('a'), makeJob('c')] })
    expect(result.current.focusedJobId).toBe('c')
    expect(result.current.focusedIndex).toBe(1)
  })

  it('clamps to the last item when the list shrinks below the prior index', () => {
    const { result, rerender } = renderHook(({ jobs }) => useFocusedJob(jobs), {
      initialProps: { jobs: JOBS },
    })
    act(() => result.current.focusLast())
    expect(result.current.focusedIndex).toBe(2)

    rerender({ jobs: [makeJob('a')] })
    expect(result.current.focusedJobId).toBe('a')
    expect(result.current.focusedIndex).toBe(0)
  })

  it('resetKey change forces focus back to the first item', () => {
    const { result, rerender } = renderHook(
      ({ jobs, key }) => useFocusedJob(jobs, { resetKey: key }),
      {
        initialProps: { jobs: JOBS, key: 'tab=new' },
      }
    )
    act(() => result.current.focusLast())
    expect(result.current.focusedJobId).toBe('c')

    rerender({ jobs: JOBS, key: 'tab=saved' })
    expect(result.current.focusedJobId).toBe('a')
    expect(result.current.focusedIndex).toBe(0)
  })

  it('adopts the first item when the list goes from empty to populated', () => {
    const { result, rerender } = renderHook(({ jobs }) => useFocusedJob(jobs), {
      initialProps: { jobs: [] as Job[] },
    })
    expect(result.current.focusedJobId).toBeNull()
    rerender({ jobs: JOBS })
    expect(result.current.focusedJobId).toBe('a')
  })

  it('drops focus to null when the list becomes empty', () => {
    const { result, rerender } = renderHook(({ jobs }) => useFocusedJob(jobs), {
      initialProps: { jobs: JOBS },
    })
    rerender({ jobs: [] as Job[] })
    expect(result.current.focusedJobId).toBeNull()
    expect(result.current.focusedIndex).toBe(-1)
  })
})
