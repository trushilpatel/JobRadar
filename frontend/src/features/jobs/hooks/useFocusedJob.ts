import { useCallback, useState } from 'react'
import type { Job } from '@/types'

export interface UseFocusedJobOptions {
  /**
   * Stable key that, when changed, resets focus to the first item.
   * Use for deliberate context changes (tab change, page change,
   * filter change). When omitted, focus is preserved as long as the
   * focused job's id is still in the list.
   */
  resetKey?: string
}

export interface UseFocusedJobReturn {
  focusedJobId: string | null
  focusedIndex: number
  focusedJob: Job | null
  focusNext: () => void
  focusPrev: () => void
  focusFirst: () => void
  focusLast: () => void
  setFocusedJobId: (id: string | null) => void
  setFocusedIndex: (idx: number) => void
}

/**
 * Keyboard-focused row tracker for the jobs page.
 *
 * Focus is tracked by job id, not array index. A TanStack Query refetch
 * (which returns a new array reference) does NOT bounce the cursor back to
 * row 0 — the same job stays focused as long as its id is still in the list.
 * When the focused job is removed (dismissed, filtered out), focus clamps to
 * the same index position so the cursor naturally lands on the next row.
 *
 * Pass `resetKey` to opt in to a hard reset on deliberate context changes
 * (tab / page / filter). When the key changes, focus jumps to the first item.
 */
export function useFocusedJob(jobs: Job[], options?: UseFocusedJobOptions): UseFocusedJobReturn {
  const resetKey = options?.resetKey

  const [focusedJobId, setFocusedJobId] = useState<string | null>(() => jobs[0]?.id ?? null)

  // Hard reset on deliberate resetKey change.
  const [prevResetKey, setPrevResetKey] = useState<string | undefined>(resetKey)
  if (prevResetKey !== resetKey) {
    setPrevResetKey(resetKey)
    setFocusedJobId(jobs[0]?.id ?? null)
  }

  // Auto-adopt the first item ONLY on empty→populated transition. We do not
  // adopt on every render where focus happens to be null (that would override
  // the user's explicit setFocusedJobId(null)).
  const [prevHadJobs, setPrevHadJobs] = useState(jobs.length > 0)
  const currentlyHasJobs = jobs.length > 0
  if (prevHadJobs !== currentlyHasJobs) {
    setPrevHadJobs(currentlyHasJobs)
    if (currentlyHasJobs && focusedJobId === null) {
      setFocusedJobId(jobs[0]?.id ?? null)
    } else if (!currentlyHasJobs && focusedJobId !== null) {
      setFocusedJobId(null)
    }
  }

  // Track previous index so that when the focused job disappears we can
  // clamp to the same position instead of bouncing to 0. Stored as state
  // (not a ref) because the React Compiler lint rule disallows ref reads
  // during render — and the prior-state-during-render pattern is what the
  // React docs prescribe for derived state.
  const [priorIndex, setPriorIndex] = useState(0)

  let focusedIndex = focusedJobId ? jobs.findIndex((j) => j.id === focusedJobId) : -1

  if (focusedIndex !== -1) {
    if (priorIndex !== focusedIndex) setPriorIndex(focusedIndex)
  } else if (focusedJobId !== null && jobs.length > 0) {
    // Focused job is gone but list is non-empty — clamp.
    const clampedIdx = Math.min(priorIndex, jobs.length - 1)
    const replacement = jobs[clampedIdx]?.id ?? null
    if (replacement !== focusedJobId) {
      setFocusedJobId(replacement)
    }
    focusedIndex = clampedIdx
  }

  const focusedJob: Job | null = focusedIndex >= 0 ? (jobs[focusedIndex] ?? null) : null

  // Use functional setState so a batch of focusNext/focusPrev calls within a
  // single render tick still walk the cursor correctly.
  const focusNext = useCallback(() => {
    if (jobs.length === 0) return
    setFocusedJobId((prev) => {
      const curIdx = prev ? jobs.findIndex((j) => j.id === prev) : -1
      const nextIdx = Math.min(curIdx + 1, jobs.length - 1)
      return jobs[nextIdx]?.id ?? prev
    })
  }, [jobs])

  const focusPrev = useCallback(() => {
    if (jobs.length === 0) return
    setFocusedJobId((prev) => {
      const curIdx = prev ? jobs.findIndex((j) => j.id === prev) : 0
      const prevIdx = Math.max(curIdx - 1, 0)
      return jobs[prevIdx]?.id ?? prev
    })
  }, [jobs])

  const focusFirst = useCallback(() => {
    if (jobs.length === 0) return
    setFocusedJobId(jobs[0]?.id ?? null)
  }, [jobs])

  const focusLast = useCallback(() => {
    if (jobs.length === 0) return
    setFocusedJobId(jobs[jobs.length - 1]?.id ?? null)
  }, [jobs])

  const setFocusedJobIdSafe = useCallback(
    (id: string | null) => {
      if (id === null) {
        setFocusedJobId(null)
        return
      }
      const exists = jobs.some((j) => j.id === id)
      setFocusedJobId(exists ? id : null)
    },
    [jobs]
  )

  const setFocusedIndex = useCallback(
    (idx: number) => {
      const target = jobs[idx]
      setFocusedJobId(target?.id ?? null)
    },
    [jobs]
  )

  return {
    focusedJobId,
    focusedIndex,
    focusedJob,
    focusNext,
    focusPrev,
    focusFirst,
    focusLast,
    setFocusedJobId: setFocusedJobIdSafe,
    setFocusedIndex,
  }
}
