import { useEffect, useState } from 'react'

/**
 * Subscribes to a `window.matchMedia` query and returns the current match
 * state. Updates whenever the media query's match state changes.
 *
 * Safe under SSR — returns `false` when `window` is undefined and only
 * subscribes inside `useEffect`.
 */
export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState<boolean>(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return false
    }
    return window.matchMedia(query).matches
  })

  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return
    }
    const mq = window.matchMedia(query)
    const handler = (event: MediaQueryListEvent) => setMatches(event.matches)
    // Sync state in case the query changed between render and effect.
    setMatches(mq.matches)
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [query])

  return matches
}

// Convenience breakpoint hooks matching Tailwind defaults used in the design spec.
export const useIsMobile = () => useMediaQuery('(max-width: 639px)')
export const useIsDesktop = () => useMediaQuery('(min-width: 1024px)')
