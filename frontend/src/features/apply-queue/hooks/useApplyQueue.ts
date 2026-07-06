import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchApplyQueue } from '../api/apply-queue.api'
import { applyQueueQueryKeys } from '../queryKeys'
import { APPLY_QUEUE_FILTER, DEFAULT_APPLY_QUEUE_LIMIT, type ApplyQueueFilter } from '../constants'

// ── Cursor history helpers ────────────────────────────────────────────────────
//
// cursorHistory[i] is the cursor string that fetches page (i+1). Index 0
// always holds '' (first page, no cursor needed). Persisted to
// sessionStorage keyed by a filter+limit fingerprint so same-session
// back/forward navigation restores page data without refetching — mirrors
// features/jobs/hooks/useJobs.ts (useInfiniteQuery is forbidden for cursor
// nav in this codebase). Unlike useJobs, pages here are NOT accumulated —
// this table is prev/next, not infinite scroll.

const CURSOR_STORAGE_KEY_PREFIX = 'jobradar:apply-queue-cursor-history:'

function restoreHistory(key: string): string[] {
  try {
    const stored = sessionStorage.getItem(CURSOR_STORAGE_KEY_PREFIX + key)
    if (!stored) return ['']
    const parsed = JSON.parse(stored) as unknown
    if (Array.isArray(parsed) && parsed.every((x) => typeof x === 'string')) {
      return parsed as string[]
    }
  } catch {
    // ignore — sessionStorage may be unavailable or contain corrupt data
  }
  return ['']
}

function persistHistory(key: string, history: string[]): void {
  try {
    sessionStorage.setItem(CURSOR_STORAGE_KEY_PREFIX + key, JSON.stringify(history))
  } catch {
    // ignore — sessionStorage may be unavailable
  }
}

export function useApplyQueue() {
  const [filter, setFilter] = useState<ApplyQueueFilter>(APPLY_QUEUE_FILTER.ALL)
  const [limit, setLimit] = useState(DEFAULT_APPLY_QUEUE_LIMIT)
  const [pageIndex, setPageIndex] = useState(0)

  // Fingerprint of everything that invalidates an in-flight cursor: a cursor
  // minted for one filter/limit combination is meaningless under another.
  const filterFingerprint = useMemo(() => `${filter}:${limit}`, [filter, limit])

  const [cursorHistory, setCursorHistory] = useState<string[]>(() =>
    restoreHistory(filterFingerprint)
  )

  // Reset pagination whenever the filter or page size changes — a new
  // filter/limit combination starts a fresh first-page session. Adjusted
  // during render (not a useEffect) per the React-recommended pattern for
  // resetting state when a derived value changes — see useCompanies.ts for
  // the same pattern in this codebase.
  const [prevFingerprint, setPrevFingerprint] = useState(filterFingerprint)
  if (prevFingerprint !== filterFingerprint) {
    setPrevFingerprint(filterFingerprint)
    setCursorHistory(restoreHistory(filterFingerprint))
    setPageIndex(0)
  }

  const currentCursor = cursorHistory[pageIndex] ?? ''

  const query = useQuery({
    queryKey: applyQueueQueryKeys.list(filter, limit, currentCursor),
    queryFn: ({ signal }) => fetchApplyQueue(filter, currentCursor, limit, signal),
    staleTime: 30_000,
    // Always refetch on mount/navigation — a job's saved/applied/resume state
    // can change from the Jobs page or another tab while this page isn't
    // mounted, so trusting the 30s staleTime window on (re-)navigation would
    // show stale queue contents. staleTime is kept for within-session query
    // dedup (e.g. React StrictMode double-invoke), just not for mount gating.
    refetchOnMount: 'always',
  })

  const items = query.data?.items ?? []
  const hasNextPage = !!query.data?.next_cursor
  const hasPrevPage = pageIndex > 0
  const itemsOnPage = items.length
  const itemsBeforePage = pageIndex * limit

  // Fetches the next page — the Pagination component's "next" chevron.
  // No-op while already fetching or once the last page has been reached.
  function goToNextPage() {
    if (query.isFetching) return
    const nextCursor = query.data?.next_cursor
    if (!nextCursor) return

    const extended = cursorHistory.slice(0, pageIndex + 1).concat(nextCursor)
    setCursorHistory(extended)
    persistHistory(filterFingerprint, extended)
    setPageIndex(pageIndex + 1)
  }

  function goToPrevPage() {
    if (pageIndex > 0) setPageIndex(pageIndex - 1)
  }

  function goToFirstPage() {
    setPageIndex(0)
  }

  return {
    items,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    isError: query.isError,
    refetch: query.refetch,

    filter,
    setFilter,

    limit,
    setLimit,

    page: pageIndex + 1,
    itemsOnPage,
    itemsBeforePage,
    hasNextPage,
    hasPrevPage,
    goToNextPage,
    goToPrevPage,
    goToFirstPage,
  }
}
