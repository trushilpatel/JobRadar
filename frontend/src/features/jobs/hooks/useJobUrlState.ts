import { useCallback, useMemo } from 'react'
import {
  createSerializer,
  parseAsArrayOf,
  parseAsInteger,
  parseAsString,
  parseAsStringLiteral,
  useQueryStates,
  type inferParserType,
} from 'nuqs'
import { JOB_STATUS, TAB_KEYS, type TabKey } from '@/config/constants/jobStatus'
import { routes } from '@/config/routes'

export type LocationFilter = 'any' | 'remote' | 'onsite' | 'hybrid'
export type DateFilter = 'all' | '24h' | '7d' | '30d'
export type SortDir = 'asc' | 'desc'

const LOCATION_VALUES = ['any', 'remote', 'onsite', 'hybrid'] as const
const DATE_VALUES = ['all', '24h', '7d', '30d'] as const
const DIR_VALUES = ['asc', 'desc'] as const
const VISA_VALUES = ['any', 'true', 'false'] as const
export type VisaFilter = (typeof VISA_VALUES)[number]

const DEFAULT_TAB: TabKey = JOB_STATUS.NEW
const DEFAULT_LIMIT = 20

// Parser map — single source of truth for URL state shape.
// `clearOnDefault` (nuqs default = true) keeps default values out of the URL,
// so an unconfigured jobs page renders as `/jobs` with no query string.
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
  visa: parseAsStringLiteral(VISA_VALUES).withDefault('any'),
  /**
   * Zero-based page index. Lives in the URL so refresh and share-links land
   * on the same page within the result set. Replaces the prior local
   * `useState` and the dead `cursor` param.
   */
  page: parseAsInteger.withDefault(0),
  limit: parseAsInteger.withDefault(DEFAULT_LIMIT),
}

const serializeJobsParams = createSerializer(PARSERS)

/**
 * Build a link to the jobs page with filters pre-applied, derived from the
 * SAME parser map the page hydrates from. This is the only sanctioned way to
 * deep-link into a filtered jobs view.
 *
 * Do NOT pass filters via router `location.state` — the jobs page reads filter
 * state exclusively from the URL query string (nuqs/useJobUrlState), so any
 * `state` payload is silently dropped and the page renders unfiltered.
 *
 * Default values are omitted (clearOnDefault), so `jobsUrl({})` is `/jobs`.
 */
export function jobsUrl(filters: Partial<inferParserType<typeof PARSERS>>): string {
  return serializeJobsParams(routes.jobs(), filters)
}

// History-mode policy:
// - PUSH on big context shifts the user expects back-button to undo
//   (tab change, filter change, sort change, page-size change).
// - REPLACE on rapid / transient state (search typing, pagination clicks,
//   opening the preview pane) — those would otherwise flood browser history.
const PUSH = { history: 'push' as const }
const REPLACE = { history: 'replace' as const }

/**
 * URL-backed state for the jobs page.
 *
 * All filter / tab / pagination / preview-open state lives in the query string
 * so refresh and share-links preserve context. Local UI state (focused row,
 * preview pane width, modal flags) stays in `useState` in the page.
 *
 * The hook only owns URL state — it does not fetch. Consumers compose with
 * useJobs() to drive the actual query.
 *
 * `resetFilters` clears every filter param while preserving the active tab
 * and open preview job. That's deliberate — wiping the tab on "reset" would
 * surprise the user who only wanted to clear search/filters.
 */
export function useJobUrlState() {
  const [state, setState] = useQueryStates(PARSERS, { history: 'replace' })

  // Tab change is a big context shift — push to browser history.
  const setTab = useCallback(
    (tab: TabKey) => setState({ tab, job: null, page: 0 }, PUSH),
    [setState]
  )

  // Opening/closing the preview should NOT spam history with one entry per
  // click — keep replace.
  const setOpenJobId = useCallback(
    (id: string | null) => setState({ job: id }, REPLACE),
    [setState]
  )

  // Search updates 300ms after the last keystroke — replace, never push.
  const setQ = useCallback((q: string) => setState({ q, page: 0 }, REPLACE), [setState])

  const setCompanies = useCallback(
    (companies: string[]) => setState({ companies, page: 0 }, PUSH),
    [setState]
  )

  const setLoc = useCallback((loc: LocationFilter) => setState({ loc, page: 0 }, PUSH), [setState])

  const setExp = useCallback((exp: string[]) => setState({ exp, page: 0 }, PUSH), [setState])

  const setSince = useCallback(
    (since: DateFilter) => setState({ since, page: 0 }, PUSH),
    [setState]
  )

  const setVisa = useCallback((visa: VisaFilter) => setState({ visa, page: 0 }, PUSH), [setState])

  const setSort = useCallback((sort: string) => setState({ sort, page: 0 }, PUSH), [setState])

  const setDir = useCallback((dir: SortDir) => setState({ dir, page: 0 }, PUSH), [setState])

  const setPage = useCallback(
    (page: number) => setState({ page: Math.max(0, page) }, REPLACE),
    [setState]
  )

  const setLimit = useCallback((limit: number) => setState({ limit, page: 0 }, PUSH), [setState])

  // Atomic reset — wipes filter params while preserving the active tab and
  // open preview job. Page resets to 0 so the new (potentially empty) filter
  // shows from the top.
  const resetFilters = useCallback(() => {
    void setState(
      {
        q: '',
        companies: [],
        loc: 'any',
        exp: [],
        since: 'all',
        sort: '',
        dir: 'asc',
        page: 0,
        visa: 'any',
      },
      PUSH
    )
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
      page: state.page,
      setPage,
      limit: state.limit,
      setLimit,
      visa: state.visa,
      setVisa,
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
      state.page,
      state.limit,
      state.visa,
      setTab,
      setOpenJobId,
      setQ,
      setCompanies,
      setLoc,
      setExp,
      setSince,
      setSort,
      setDir,
      setPage,
      setLimit,
      setVisa,
      resetFilters,
    ]
  )
}
