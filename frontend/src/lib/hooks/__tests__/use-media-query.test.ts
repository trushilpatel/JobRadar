import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useMediaQuery, useIsMobile, useIsDesktop } from '@/lib/hooks/use-media-query'

type Listener = (event: MediaQueryListEvent) => void

interface FakeMediaQueryList {
  matches: boolean
  media: string
  addEventListener: (event: 'change', cb: Listener) => void
  removeEventListener: (event: 'change', cb: Listener) => void
  // Test helper to fire a change event.
  __fire: (matches: boolean) => void
}

function createFakeMQL(initial: boolean, media: string): FakeMediaQueryList {
  const listeners = new Set<Listener>()
  const mql: FakeMediaQueryList = {
    matches: initial,
    media,
    addEventListener: (_event, cb) => {
      listeners.add(cb)
    },
    removeEventListener: (_event, cb) => {
      listeners.delete(cb)
    },
    __fire: (matches: boolean) => {
      mql.matches = matches
      const evt = { matches, media } as unknown as MediaQueryListEvent
      listeners.forEach((cb) => cb(evt))
    },
  }
  return mql
}

let registry: Map<string, FakeMediaQueryList>
let originalMatchMedia: typeof window.matchMedia | undefined

beforeEach(() => {
  registry = new Map()
  originalMatchMedia = window.matchMedia
  window.matchMedia = vi.fn().mockImplementation((query: string) => {
    const existing = registry.get(query)
    if (existing) return existing
    const fresh = createFakeMQL(false, query)
    registry.set(query, fresh)
    return fresh
  }) as unknown as typeof window.matchMedia
})

afterEach(() => {
  if (originalMatchMedia) {
    window.matchMedia = originalMatchMedia
  }
})

describe('useMediaQuery', () => {
  it('returns the initial match state', () => {
    registry.set('(min-width: 1024px)', createFakeMQL(true, '(min-width: 1024px)'))
    const { result } = renderHook(() => useMediaQuery('(min-width: 1024px)'))
    expect(result.current).toBe(true)
  })

  it('returns false when initial query does not match', () => {
    const { result } = renderHook(() => useMediaQuery('(max-width: 100px)'))
    expect(result.current).toBe(false)
  })

  it('updates when the media query fires a change event', () => {
    const { result } = renderHook(() => useMediaQuery('(min-width: 800px)'))
    const mql = registry.get('(min-width: 800px)')!
    expect(result.current).toBe(false)

    act(() => {
      mql.__fire(true)
    })
    expect(result.current).toBe(true)

    act(() => {
      mql.__fire(false)
    })
    expect(result.current).toBe(false)
  })

  it('removes its change listener on unmount', () => {
    const { unmount } = renderHook(() => useMediaQuery('(min-width: 1px)'))
    const mql = registry.get('(min-width: 1px)')!
    const removeSpy = vi.spyOn(mql, 'removeEventListener')
    unmount()
    expect(removeSpy).toHaveBeenCalledTimes(1)
  })

  it('useIsMobile resolves the mobile breakpoint', () => {
    registry.set('(max-width: 639px)', createFakeMQL(true, '(max-width: 639px)'))
    const { result } = renderHook(() => useIsMobile())
    expect(result.current).toBe(true)
  })

  it('useIsDesktop resolves the desktop breakpoint', () => {
    registry.set('(min-width: 1024px)', createFakeMQL(true, '(min-width: 1024px)'))
    const { result } = renderHook(() => useIsDesktop())
    expect(result.current).toBe(true)
  })
})
