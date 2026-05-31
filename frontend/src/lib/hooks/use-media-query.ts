import { useCallback, useSyncExternalStore } from 'react'

const noop = () => () => {}

/**
 * Subscribes to a `window.matchMedia` query and returns the current match
 * state. Built on `useSyncExternalStore` so the value is always coherent
 * with the browser's media query state, including across React concurrent
 * tearing and SSR hydration.
 *
 * Safe under SSR — `getServerSnapshot` returns `false` and no subscription
 * is established until hydration.
 */
export function useMediaQuery(query: string): boolean {
  const subscribe = useCallback(
    (notify: () => void) => {
      if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
        return noop()
      }
      const mql = window.matchMedia(query)
      mql.addEventListener('change', notify)
      return () => mql.removeEventListener('change', notify)
    },
    [query]
  )

  const getSnapshot = useCallback(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return false
    }
    return window.matchMedia(query).matches
  }, [query])

  const getServerSnapshot = useCallback(() => false, [])

  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot)
}

// Convenience breakpoint hooks matching Tailwind defaults used in the design spec.
export const useIsMobile = () => useMediaQuery('(max-width: 639px)')
export const useIsDesktop = () => useMediaQuery('(min-width: 1024px)')
