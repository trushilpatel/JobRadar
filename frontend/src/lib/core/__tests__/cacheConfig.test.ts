import { describe, it, expect } from 'vitest'
import { CACHE_TTL } from '../cacheConfig'

describe('CACHE_TTL', () => {
  it('defines a longer staleTime/gcTime for resumes than the global default', () => {
    expect(CACHE_TTL.resumes.staleTime).toBe(5 * 60_000)
    expect(CACHE_TTL.resumes.gcTime).toBe(30 * 60_000)
  })
})
