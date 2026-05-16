import { useCallback, useMemo } from 'react'
import {
  parseAsArrayOf,
  parseAsInteger,
  parseAsString,
  parseAsStringLiteral,
  useQueryStates,
} from 'nuqs'
import { JOB_STATUS, TAB_KEYS, type TabKey } from '@/constants/jobStatus'

export type LocationFilter = 'any' | 'remote' | 'onsite'
export type DateFilter = 'all' | '24h' | '7d' | '30d'
export type SortDir = 'asc' | 'desc'

const LOCATION_VALUES = ['any', 'remote', 'onsite'] as const
const DATE_VALUES = ['all', '24h', '7d', '30d'] as const
const DIR_VALUES = ['asc', 'desc'] as const

const DEFAULT_TAB: TabKey = JOB_STATUS.NEW
const DEFAULT_LIMIT = 20

// Parser map — single source of truth for URL state shape.
// `clearOnDefault` (nuqs default = true) means we don't pollute the URL with
// default values; the URL stays clean when no filters are active.
const PARSERS = {
  tab: parseAsStringLiteral(TAB_KEYS).withDefault(DEFAULT_TAB),
  job: parseAsString,
  q: parseAsString.withDefault(''),
  companies: parseAsArrayOf(parseAsString).withDefault([]),
  loc: parseAsStringLiteral(LOCATION_VALUES).withDefault('any'),
  exp: parseAsArrayOf(parseAsString).withDefault([]),
  since: parseAsStringLiteral(DATE_VALUES).withDefault('all'),
  sort: parseAsString.withDefault(''),
  dir: parseAsStringLiteral(DIR_VALUES).withDefault('asc'),
  cursor: parseAsString.withDefault(''),
  limit: parseAsInteger.withDefault(DEFAULT_LIMIT),
}

/**
 * URL-backed state for the jobs page.
 *
 * All filter / tab / pagination / preview-open state lives in the query string
 * so refresh and share-links preserve context. Local UI state (focused row,
 * preview pane width, modal flags) stays in `useState` in the page.
 *
 * The hook only owns URL state — it does not fetch. Consumers compose with
 * useJobs() to drive the actual query.
 */
export function useJobUrlState() {
  const [state, setState] = useQueryStates(PARSERS, {
    history: 'replace',
  })

  // Atomic setter for the tab — clears the preview job and pagination cursor
  // since those values are scoped to the previous tab's listing.
  const setTab = useCallback(
    (tab: TabKey) => {
      return setState({ tab, job: null, cursor: '' })
    },
    [setState]
  )

  const setOpenJobId = useCallback(
    (id: string | null) => {
      return setState({ job: id })
    },
    [setState]
  )

  const setQ = useCallback(
    (q: string) => {
      // Reset the cursor when search changes — the previous cursor no longer
      // maps to a meaningful position in the new result set.
      return setState({ q, cursor: '' })
    },
    [setState]
  )

  const setCompanies = useCallback(
    (companies: string[]) => {
      return setState({ companies, cursor: '' })
    },
    [setState]
  )

  const setLoc = useCallback(
    (loc: LocationFilter) => {
      return setState({ loc, cursor: '' })
    },
    [setState]
  )

  const setExp = useCallback(
    (exp: string[]) => {
      return setState({ exp, cursor: '' })
    },
    [setState]
  )

  const setSince = useCallback(
    (since: DateFilter) => {
      return setState({ since, cursor: '' })
    },
    [setState]
  )

  const setSort = useCallback(
    (sort: string) => {
      return setState({ sort, cursor: '' })
    },
    [setState]
  )

  const setDir = useCallback(
    (dir: SortDir) => {
      return setState({ dir, cursor: '' })
    },
    [setState]
  )

  const setCursor = useCallback(
    (cursor: string) => {
      return setState({ cursor })
    },
    [setState]
  )

  const setLimit = useCallback(
    (limit: number) => {
      return setState({ limit, cursor: '' })
    },
    [setState]
  )

  // Atomic reset — wipes every filter param while preserving the active tab and
  // open preview job. Pagination cursor also resets so the new (empty) filter
  // shows from the top.
  const resetFilters = useCallback(() => {
    void setState({
      q: '',
      companies: [],
      loc: 'any',
      exp: [],
      since: 'all',
      sort: '',
      dir: 'asc',
      cursor: '',
    })
  }, [setState])

  return useMemo(
    () => ({
      tab: state.tab,
      setTab,
      openJobId: state.job,
      setOpenJobId,
      q: state.q,
      setQ,
      companies: state.companies,
      setCompanies,
      loc: state.loc,
      setLoc,
      exp: state.exp,
      setExp,
      since: state.since,
      setSince,
      sort: state.sort,
      setSort,
      dir: state.dir,
      setDir,
      cursor: state.cursor,
      setCursor,
      limit: state.limit,
      setLimit,
      resetFilters,
    }),
    [
      state.tab,
      state.job,
      state.q,
      state.companies,
      state.loc,
      state.exp,
      state.since,
      state.sort,
      state.dir,
      state.cursor,
      state.limit,
      setTab,
      setOpenJobId,
      setQ,
      setCompanies,
      setLoc,
      setExp,
      setSince,
      setSort,
      setDir,
      setCursor,
      setLimit,
      resetFilters,
    ]
  )
}
