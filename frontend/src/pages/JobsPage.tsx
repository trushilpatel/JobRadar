import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import { Keyboard } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import type { Job, JobStatus } from '@/types'
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
import { ResizeHandle } from '@/components/ui/resize-handle'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import type { TabKey } from '@/config/constants/jobStatus'
import {
  useJobs,
  useJobActions,
  useJobKeyboard,
  useFocusedJob,
  JobStatusTabs,
  JobFilters,
  JobsTable,
  JobDetailPanel,
  JobCommandPalette,
  JobShortcutSheet,
} from '@/features/jobs'
import { useIsMobile } from '@/lib/hooks/use-media-query'

type SettableStatus = Exclude<JobStatus, 'dismissed' | 'new' | 'saved'>

const SCROLL_RESTORATION_KEY = 'jobs-page-scroll-y'
const DEFAULT_PANEL_WIDTH = 480

export function JobsPage() {
  const jobs = useJobs()
  const actions = useJobActions()
  const navigate = useNavigate()
  const isMobile = useIsMobile()

  // Local UI state — overlay flags. None of these survive a refresh by design.
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [helpOpen, setHelpOpen] = useState(false)
  const [mobileFilterOpen, setMobileFilterOpen] = useState(false)
  // User can close the always-open detail panel; honored until they click a
  // row or change tabs.
  const [panelClosedByUser, setPanelClosedByUser] = useState(false)
  const [panelWidth, setPanelWidth] = useState(DEFAULT_PANEL_WIDTH)

  const visibleJobs = jobs.jobs

  // Focused-row tracking. Keyed by tab+filter+page so deliberate context
  // switches reset focus to the first item; otherwise focus follows the
  // same job id across refetches.
  const focusResetKey = jobs.tab
  const focus = useFocusedJob(visibleJobs, { resetKey: focusResetKey })

  // ── Open-job lifecycle ─────────────────────────────────────────────────────
  // openJob is URL-driven (?job=). If the user changes filters and the open
  // job is no longer in the result set, auto-clear the URL param rather than
  // leaving a stale `?job=<gone>` that a refresh would silently fail to open.
  const openJob = useMemo(
    () => visibleJobs.find((j) => j.id === jobs.openJobId) ?? null,
    [visibleJobs, jobs.openJobId]
  )
  useEffect(() => {
    if (jobs.openJobId && !openJob && !jobs.isFetching) {
      jobs.setOpenJobId(null)
    }
  }, [jobs, openJob])

  // ── Auto-open the focused row's detail panel on desktop ────────────────────
  // The panel is always-open by default (Gmail/Superhuman pattern). Arrow keys
  // move focus AND the previewed job in lockstep. The user can explicitly
  // close the panel via the X button; that intent persists until they pick a
  // new row or change context.
  useEffect(() => {
    if (isMobile) return
    if (panelClosedByUser) return
    const target = focus.focusedJob
    if (!target) return
    if (jobs.openJobId !== target.id) {
      jobs.setOpenJobId(target.id)
    }
  }, [focus.focusedJob, jobs, isMobile, panelClosedByUser])

  // ── Scroll restoration ─────────────────────────────────────────────────────
  const tableScrollRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const el = tableScrollRef.current
    if (!el) return
    const stored = Number(sessionStorage.getItem(SCROLL_RESTORATION_KEY) ?? '0')
    if (stored > 0) el.scrollTop = stored
    return () => {
      sessionStorage.setItem(SCROLL_RESTORATION_KEY, String(el.scrollTop))
    }
  }, [])

  // ── Action handlers ───────────────────────────────────────────────────────
  const handleStatusChange = useCallback(
    (job: Job, status: Exclude<JobStatus, 'dismissed' | 'saved'>) => {
      actions.setStatus.mutate({ id: job.id, status })
      if (jobs.openJobId === job.id) jobs.setOpenJobId(null)
    },
    [actions.setStatus, jobs]
  )

  // The bookmark column wires a one-click save toggle. Saved is an
  // ORTHOGONAL flag (saved_at on user_job_state), NOT a workflow status —
  // toggling it never moves the job out of its current tab.
  //
  // pendingSaveIdsRef guards against a rapid double-click: `job` is a
  // render-time snapshot, so two clicks fired before either mutation settles
  // both compute the SAME target boolean from the same stale saved_at
  // (there's no re-render between them to refresh it) — the second click
  // would silently re-send the first's value instead of toggling it back.
  // Ignoring a click for a job that already has a save toggle in flight
  // turns that into "first click wins" instead of a duplicate no-op.
  const pendingSaveIdsRef = useRef<Set<string>>(new Set())
  const handleToggleSave = useCallback(
    (job: Job) => {
      if (pendingSaveIdsRef.current.has(job.id)) return
      pendingSaveIdsRef.current.add(job.id)
      const currentlySaved = job.user_state?.saved_at != null
      actions.setSaved.mutate(
        { id: job.id, saved: !currentlySaved },
        { onSettled: () => pendingSaveIdsRef.current.delete(job.id) }
      )
    },
    [actions.setSaved]
  )

  const handleOpenFocused = useCallback(() => {
    const job = focus.focusedJob
    if (!job) return
    if (isMobile) {
      navigate(`/jobs/${job.id}`)
    } else {
      setPanelClosedByUser(false)
      jobs.setOpenJobId(job.id)
    }
  }, [focus.focusedJob, isMobile, navigate, jobs])

  const handleOpenUrlFocused = useCallback(() => {
    const job = focus.focusedJob
    if (!job?.url) return
    window.open(job.url, '_blank', 'noreferrer')
  }, [focus.focusedJob])

  const handleApply = useCallback(
    (job: Job) => {
      actions.applyJob.mutate({ id: job.id, url: job.url })
    },
    [actions.applyJob]
  )

  const handleEscape = useCallback(() => {
    if (jobs.openJobId) {
      setPanelClosedByUser(true)
      jobs.setOpenJobId(null)
    }
  }, [jobs])

  const handleGoToTab = useCallback(
    (tab: TabKey) => {
      jobs.setTab(tab)
      setPanelClosedByUser(false)
    },
    [jobs]
  )

  const handleKeyboardSetStatus = useCallback(
    (status: SettableStatus) => {
      if (!focus.focusedJob) return
      handleStatusChange(focus.focusedJob, status)
    },
    [focus.focusedJob, handleStatusChange]
  )

  const handleKeyboardToggleSave = useCallback(() => {
    if (!focus.focusedJob) return
    handleToggleSave(focus.focusedJob)
  }, [focus.focusedJob, handleToggleSave])

  const searchInputRef = useRef<HTMLInputElement | null>(null)
  const handleFocusSearch = useCallback(() => {
    searchInputRef.current?.focus()
  }, [])

  // ── Keyboard wiring ───────────────────────────────────────────────────────
  const keyboardEnabled = !isMobile && !paletteOpen && !helpOpen && !mobileFilterOpen
  useJobKeyboard({
    enabled: keyboardEnabled,
    onFocusNext: focus.focusNext,
    onFocusPrev: focus.focusPrev,
    onFocusFirst: focus.focusFirst,
    onFocusLast: focus.focusLast,
    onOpenPreview: handleOpenFocused,
    onOpenUrl: handleOpenUrlFocused,
    onEscape: handleEscape,
    onGoToTab: handleGoToTab,
    onSetStatus: handleKeyboardSetStatus,
    onToggleSave: handleKeyboardToggleSave,
    onUndo: actions.undoLast,
    onFocusSearch: handleFocusSearch,
    onOpenPalette: () => setPaletteOpen(true),
    onResetFilters: jobs.resetFilters,
    onOpenHelp: () => setHelpOpen(true),
  })

  const activeFilterCount = [
    jobs.searchQuery.length > 0,
    jobs.selectedCompanies.length > 0,
    jobs.locationFilter !== 'any',
    jobs.expFilter.length > 0,
    jobs.dateFilter !== 'all',
    jobs.visaFilter !== 'any',
  ].filter(Boolean).length

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <JobStatusTabs
        tab={jobs.tab}
        onChange={(t) => {
          jobs.setTab(t)
          setPanelClosedByUser(false)
        }}
        counts={jobs.counts}
        trackingStages={jobs.trackingStages}
        onTrackingStagesChange={jobs.setTrackingStages}
        leftSlot={
          <div className="flex items-baseline gap-3">
            <h1 className="text-lg font-semibold text-primary">Jobs</h1>
            <span className="text-xs text-muted">
              {activeFilterCount > 0 && (
                <span style={{ color: 'var(--violet-bright)' }}>
                  {activeFilterCount} filter{activeFilterCount !== 1 ? 's' : ''} active
                </span>
              )}
            </span>
          </div>
        }
        rightSlot={
          !isMobile ? (
            <TooltipProvider delayDuration={150}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    aria-label="Keyboard shortcuts"
                    onClick={() => setHelpOpen(true)}
                    className="flex items-center justify-center h-7 w-7 rounded-md text-muted hover:text-primary hover:bg-hover transition-colors"
                  >
                    <Keyboard size={16} />
                  </button>
                </TooltipTrigger>
                <TooltipContent side="bottom" align="end">
                  <div className="text-[11px] text-secondary">
                    Press <span className="font-mono">?</span> for shortcuts
                  </div>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          ) : null
        }
      />

      <JobFilters
        ref={searchInputRef}
        searchQuery={jobs.searchQuery}
        onSearchChange={jobs.setSearchQuery}
        companies={jobs.companies}
        selectedCompanies={jobs.selectedCompanies}
        onCompaniesChange={jobs.setSelectedCompanies}
        locationFilter={jobs.locationFilter}
        onLocationChange={jobs.setLocationFilter}
        expFilter={jobs.expFilter}
        onExpChange={jobs.setExpFilter}
        dateFilter={jobs.dateFilter}
        onDateChange={jobs.setDateFilter}
        visaFilter={jobs.visaFilter}
        onVisaChange={jobs.setVisaFilter}
        onReset={jobs.resetFilters}
        hasActiveFilters={activeFilterCount > 0}
        mobileOpen={mobileFilterOpen}
        onMobileOpenChange={setMobileFilterOpen}
        activeFilterCount={activeFilterCount}
      />

      {/* Content: table + always-open detail panel. The panel is a flex
          sibling of the table (not a portal/overlay) so the table genuinely
          reflows to make room — same pattern as CompaniesPage's drawer. */}
      <ErrorBoundary>
        <div className="flex-1 flex min-h-0 overflow-hidden">
          <div ref={tableScrollRef} className="flex flex-1 min-w-0 overflow-hidden">
            <JobsTable
              jobs={visibleJobs}
              loading={jobs.isLoading}
              isFetchingMore={jobs.isFetchingMore}
              isError={jobs.isError}
              onRetry={() => void jobs.refetch()}
              tab={jobs.tab}
              hasNextPage={jobs.hasNextPage}
              hasActiveFilters={activeFilterCount > 0}
              activeJobId={jobs.openJobId ?? null}
              focusedIndex={focus.focusedIndex}
              onRowClick={(job) => {
                focus.setFocusedJobId(job.id)
                if (isMobile) {
                  navigate(`/jobs/${job.id}`)
                } else {
                  setPanelClosedByUser(false)
                  jobs.setOpenJobId(job.id)
                }
              }}
              onToggleSave={handleToggleSave}
              onApply={handleApply}
              getAttachmentState={() => null}
              onLoadMore={jobs.loadMore}
              sorting={jobs.sorting}
              onSortingChange={jobs.handleSortingChange}
            />
          </div>

          {!isMobile && openJob && !panelClosedByUser && (
            <>
              <ResizeHandle width={panelWidth} onChange={setPanelWidth} />
              <aside
                style={{ ['--panel-width' as string]: `${panelWidth}px` } as CSSProperties}
                className="flex flex-col shrink-0 min-h-0 h-full border-l border-[var(--border)] overflow-hidden shadow-[0_8px_24px_rgba(0,0,0,0.4)] [width:var(--panel-width)]"
              >
                <JobDetailPanel
                  job={openJob}
                  onClose={() => {
                    setPanelClosedByUser(true)
                    jobs.setOpenJobId(null)
                  }}
                  onStatusChange={handleStatusChange}
                  onToggleSave={handleToggleSave}
                />
              </aside>
            </>
          )}
        </div>
      </ErrorBoundary>

      <JobCommandPalette
        open={paletteOpen}
        onOpenChange={setPaletteOpen}
        recentJobs={visibleJobs}
        onGoToTab={handleGoToTab}
        onSelectJob={(job) => {
          if (isMobile) navigate(`/jobs/${job.id}`)
          else {
            setPanelClosedByUser(false)
            jobs.setOpenJobId(job.id)
          }
        }}
        onResetFilters={jobs.resetFilters}
        onOpenHelp={() => setHelpOpen(true)}
      />
      <JobShortcutSheet open={helpOpen} onOpenChange={setHelpOpen} />
    </div>
  )
}
