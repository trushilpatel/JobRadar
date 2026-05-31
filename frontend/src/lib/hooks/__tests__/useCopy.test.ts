import { describe, it, expect } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useCopy } from '@/lib/hooks/useCopy'

describe('useCopy', () => {
  it('returns copy for a nav key', () => {
    const { result } = renderHook(() => useCopy('nav.dashboard'))
    expect(result.current).toBe('Dashboard')
  })

  it('returns copy for a job tab key', () => {
    const { result } = renderHook(() => useCopy('jobs.tab.saved'))
    expect(result.current).toBe('Saved')
  })

  it('returns the search placeholder copy', () => {
    const { result } = renderHook(() => useCopy('jobs.search.placeholder'))
    expect(result.current).toBe('Search jobs…')
  })
})
