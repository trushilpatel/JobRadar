import { useCallback, useState } from 'react'
import type { Job } from '@/types'

/**
 * Tracks the index of the currently focused row in a job list. Distinct from
 * the "open preview" job (which lives in URL state via useJobUrlState.job).
 *
 * Focus moves with j/k/arrow shortcuts and is what status keys act on.
 * Pressing Enter promotes the focused row to the open preview.
 *
 * Focus resets to 0 whenever the underlying job list reference changes
 * (filter/tab/page change). We compare the previous reference during render
 * — the React-recommended alternative to a `setState` in `useEffect` for
 * derived state — so the cursor never points at a stale row.
 */
export function useFocusedJob(jobs: Job[]) {
  const [focusedIndex, setFocusedIndex] = useState(0)
  const [prevJobs, setPrevJobs] = useState(jobs)
  if (prevJobs !== jobs) {
    setPrevJobs(jobs)
    setFocusedIndex(0)
  }

  const focusNext = useCallback(() => {
    setFocusedIndex((i) => Math.min(i + 1, Math.max(jobs.length - 1, 0)))
  }, [jobs.length])

  const focusPrev = useCallback(() => {
    setFocusedIndex((i) => Math.max(i - 1, 0))
  }, [])

  const focusFirst = useCallback(() => setFocusedIndex(0), [])

  const focusLast = useCallback(() => setFocusedIndex(Math.max(jobs.length - 1, 0)), [jobs.length])

  const focusedJob: Job | null = jobs[focusedIndex] ?? null

  return {
    focusedIndex,
    focusedJob,
    focusNext,
    focusPrev,
    focusFirst,
    focusLast,
    setFocusedIndex,
  }
}
