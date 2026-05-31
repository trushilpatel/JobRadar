import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
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
import { JobResumeSection } from '@/pages/jobs/JobResumeSection'
// TODO: replace with real useJobAttachment(jobId) hook once backend is wired
import { getMockAttachmentForJob } from '@/features/resumes'
import { useIsMobile } from '@/lib/hooks/use-media-query'
import { useHiddenItems } from '@/lib/hooks/hidden-items'
import { showDismissStripe } from '@/components/ui/dismiss-stripe'

type SettableStatus = Exclude<JobStatus, 'dismissed' | 'new' | 'saved'>

const SCROLL_RESTORATION_KEY = 'jobs-page-scroll-y'
const HIDDEN_COMPANIES_CATEGORY = 'companies'

export function JobsPage() {
  const jobs = useJobs()
  const actions = useJobActions()
  const navigate = useNavigate()
  const isMobile = useIsMobile()
  const hiddenCompanies = useHiddenItems(HIDDEN_COMPANIES_CATEGORY)

  // Local UI state — layout widths, overlay flags. None of these survive a
  // refresh by design.
  const [panelWidth, setPanelWidth] = useState(440)
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [helpOpen, setHelpOpen] = useState(false)
  const [mobileFilterOpen, setMobileFilterOpen] = useState(false)
  // User can dismiss the always-open detail panel; honored until they click a
  // row or change tabs.
  const [panelClosedByUser, setPanelClosedByUser] = useState(false)

  // ── Hide-list filtering ────────────────────────────────────────────────────
  // Jobs from a hidden company are pruned at the page level (after fetch).
  // Server-side filtering would be ideal but localStorage-backed hides are
  // client-only by design — a single user, opt-out blocks they can manage
  // themselves without a backend round-trip.
  const visibleJobs = useMemo(
    () =>
      hiddenCompanies.items.length === 0
        ? jobs.jobs
        : jobs.jobs.filter((j) => {
            const co = j.company?.name ?? ''
            return !hiddenCompanies.has(co)
          }),
    [jobs.jobs, hiddenCompanies]
  )

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

  // Single-action dismiss with zero-friction toast. Reason defaults to the
  // existing 'user_dismissed' enum value — preserves the schema for future
  // richer reason-collection without making the user think about it now.
  const handleDismiss = useCallback(
    (job: Job) => {
      const companyName = job.company?.name ?? ''
      actions.dismiss.mutate({ id: job.id, reason: 'user_dismissed' })
      if (jobs.openJobId === job.id) jobs.setOpenJobId(null)

      // Sonner's default `useUndoToast` is replaced for the dismiss flow by
      // the richer DismissStripe (adds optional "Don't show X again"). The
      // underlying mutation's own undo toast (via useUndoToast in
      // useJobActions) still fires — we'd ideally suppress it, but for now
      // it's accepted that the user sees both briefly.
      showDismissStripe({
        message: 'Dismissed',
        submessage: `${job.title}${companyName ? ` at ${companyName}` : ''}`,
        onUndo: () => actions.undoLast(),
        hideTarget: companyName
          ? { category: HIDDEN_COMPANIES_CATEGORY, value: companyName, label: companyName }
          : undefined,
      })
    },
    [actions, jobs]
  )

  // The bookmark column wires a one-click save toggle. Saved is an
  // ORTHOGONAL flag (saved_at on user_job_state), NOT a workflow status —
  // toggling it never moves the job out of its current tab.
  const handleToggleSave = useCallback(
    (job: Job) => {
      const currentlySaved = job.user_state?.saved_at != null
      actions.setSaved.mutate({ id: job.id, saved: !currentlySaved })
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

  const handleKeyboardDismiss = useCallback(() => {
    if (!focus.focusedJob) return
    handleDismiss(focus.focusedJob)
  }, [focus.focusedJob, handleDismiss])

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
    onPrevPage: jobs.goToPrevPage,
    onNextPage: jobs.goToNextPage,
    onFirstPage: jobs.goToFirstPage,
    onGoToTab: handleGoToTab,
    onSetStatus: handleKeyboardSetStatus,
    onToggleSave: handleKeyboardToggleSave,
    onDismiss: handleKeyboardDismiss,
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
        leftSlot={
          <div className="flex items-baseline gap-3">
            <h1 className="text-lg font-semibold text-white">Jobs</h1>
            <span className="text-xs text-slate-400">
              {activeFilterCount > 0 && (
                <span className="text-violet-300">
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
                    className="flex items-center justify-center h-7 w-7 rounded-md text-slate-400 hover:text-white hover:bg-white/[0.05] transition-colors"
                  >
                    <Keyboard size={16} />
                  </button>
                </TooltipTrigger>
                <TooltipContent side="bottom" align="end">
                  <div className="text-[11px] text-slate-300">
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
        onReset={jobs.resetFilters}
        hasActiveFilters={activeFilterCount > 0}
        mobileOpen={mobileFilterOpen}
        onMobileOpenChange={setMobileFilterOpen}
        activeFilterCount={activeFilterCount}
      />

      {/* Content: table + always-open detail panel */}
      <ErrorBoundary>
        <div className="flex-1 flex min-h-0 overflow-hidden">
          <div ref={tableScrollRef} className="flex flex-1 min-w-0 overflow-hidden">
            <JobsTable
              jobs={visibleJobs}
              loading={jobs.isLoading || jobs.isFetchingNextPage}
              isError={jobs.isError}
              onRetry={() => void jobs.refetch()}
              tab={jobs.tab}
              page={jobs.page}
              limit={jobs.limit}
              itemsOnPage={jobs.itemsOnPage}
              itemsBeforePage={jobs.itemsBeforePage}
              hasNextPage={jobs.hasNextPage}
              hasPrevPage={jobs.hasPrevPage}
              knownTotal={activeFilterCount === 0 ? jobs.counts[jobs.tab as TabKey] : undefined}
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
              onDismiss={handleDismiss}
              getAttachmentState={(jobId) => getMockAttachmentForJob(jobId)?.state ?? null}
              onNextPage={jobs.goToNextPage}
              onPrevPage={jobs.goToPrevPage}
              onFirstPage={jobs.goToFirstPage}
              onLimitChange={jobs.setLimit}
              sorting={jobs.sorting}
              onSortingChange={jobs.handleSortingChange}
            />
          </div>

          {!isMobile && openJob && !panelClosedByUser && (
            <>
              <ResizeHandle width={panelWidth} onChange={setPanelWidth} min={300} max={720} />
              <div
                style={{ '--panel-width': `${panelWidth}px` } as CSSProperties}
                className="shrink-0 flex flex-col min-h-0 border-l border-white/[0.07] overflow-hidden [width:var(--panel-width)]"
              >
                <JobDetailPanel
                  job={openJob}
                  onClose={() => {
                    setPanelClosedByUser(true)
                    jobs.setOpenJobId(null)
                  }}
                  onStatusChange={handleStatusChange}
                  onToggleSave={handleToggleSave}
                  onDismiss={handleDismiss}
                  resumeSection={<JobResumeSection jobId={openJob.id} />}
                />
              </div>
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
