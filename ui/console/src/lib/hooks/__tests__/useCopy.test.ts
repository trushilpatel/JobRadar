import { describe, it, expect } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useCopy } from '@/lib/hooks/useCopy'

describe('useCopy', () => {
  it('returns the copy string for a nav key', () => {
    const { result } = renderHook(() => useCopy('nav.dashboard'))
    expect(result.current).toBe('Dashboard')
  })

  it('returns the copy string for a jobs key', () => {
    const { result } = renderHook(() => useCopy('jobs.save'))
    expect(result.current).toBe('Save')
  })
})
