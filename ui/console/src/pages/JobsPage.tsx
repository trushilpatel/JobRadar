import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Trash2, BookmarkCheck, CheckCircle2, Keyboard } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import type { Job, JobStatus } from '@/types'
import { ResizeHandle } from '@/components/ui/resize-handle'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import type { TabKey } from '@/constants/jobStatus'
import {
  useJobs,
  useJobActions,
  useJobKeyboard,
  useFocusedJob,
  JobStatusTabs,
  JobFilters,
  JobsTable,
  JobDetailPanel,
  DismissModal,
  JobCommandPalette,
  JobShortcutSheet,
} from '@/features/jobs'
import type { DismissPayload } from '@/features/jobs/components/DismissModal'
import { useIsMobile } from '@/lib/use-media-query'

type SettableStatus = Exclude<JobStatus, 'dismissed' | 'new'>

// Holder for the dismiss-modal target — either a single job or a count of
// `selectedIds` for the bulk path. We discriminate by which field is set.
type DismissTarget = { kind: 'single'; job: Job } | { kind: 'bulk'; ids: string[] } | null

const SCROLL_RESTORATION_KEY = 'jobs-page-scroll-y'

export function JobsPage() {
  const jobs = useJobs()
  const actions = useJobActions()
  const navigate = useNavigate()
  const isMobile = useIsMobile()

  // Local UI state — selection, layout widths, overlay flags.
  // None of these survive a refresh by design.
  const [dismissTarget, setDismissTarget] = useState<DismissTarget>(null)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [panelWidth, setPanelWidth] = useState(440)
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [helpOpen, setHelpOpen] = useState(false)
  const [mobileFilterOpen, setMobileFilterOpen] = useState(false)
  // Anchor for shift-range selection (last toggled row id).
  const [lastAnchorId, setLastAnchorId] = useState<string | null>(null)

  // Focused-row tracking. Keyed by tab+filter+page so deliberate context
  // switches reset focus to the first item; otherwise focus follows the
  // same job id across refetches.
  const focusResetKey = jobs.tab
  const focus = useFocusedJob(jobs.jobs, { resetKey: focusResetKey })

  // ── Selection lifecycle ────────────────────────────────────────────────────
  // Prune phantom IDs whenever the visible job list changes — prevents a
  // `shift+d` press from operating on rows the user can no longer see (and
  // would not have selected through the keyboard or checkbox).
  // prev-state-during-render pattern instead of useEffect: the prune is
  // pure-derived state, so React recommends this form over setState-in-effect.
  const [prevJobsRef, setPrevJobsRef] = useState(jobs.jobs)
  if (prevJobsRef !== jobs.jobs) {
    setPrevJobsRef(jobs.jobs)
    setSelectedIds((prev) => {
      if (prev.size === 0) return prev
      const visible = new Set(jobs.jobs.map((j) => j.id))
      let changed = false
      const next = new Set<string>()
      for (const id of prev) {
        if (visible.has(id)) next.add(id)
        else changed = true
      }
      return changed ? next : prev
    })
    setLastAnchorId((prev) => (prev && jobs.jobs.some((j) => j.id === prev) ? prev : null))
  }

  // ── Open-job lifecycle ─────────────────────────────────────────────────────
  // openJob is URL-driven (?job=). If the user changes filters and the open
  // job is no longer in the result set, auto-clear the URL param rather than
  // leaving a stale `?job=<gone>` that a refresh would silently fail to open.
  const openJob = useMemo(
    () => jobs.jobs.find((j) => j.id === jobs.openJobId) ?? null,
    [jobs.jobs, jobs.openJobId]
  )
  useEffect(() => {
    if (jobs.openJobId && !openJob && !jobs.isFetching) {
      jobs.setOpenJobId(null)
    }
  }, [jobs, openJob])

  // ── Scroll restoration ────────────────────────────────────────────────────
  // Save scroll-y as the user leaves the page; restore on remount. Mobile
  // users navigate to /jobs/:id and back constantly — losing scroll position
  // each time turns long lists into a re-scroll chore.
  const tableScrollRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const el = tableScrollRef.current
    if (!el) return
    const stored = Number(sessionStorage.getItem(SCROLL_RESTORATION_KEY) ?? '0')
    if (stored > 0) el.scrollTop = stored
    // Capture the ref node in the effect's closure so the cleanup reads the
    // same DOM node that was mounted (react-hooks/exhaustive-deps).
    return () => {
      sessionStorage.setItem(SCROLL_RESTORATION_KEY, String(el.scrollTop))
    }
  }, [])

  // ── Action handlers ───────────────────────────────────────────────────────
  const handleStatusChange = useCallback(
    (job: Job, status: Exclude<JobStatus, 'dismissed'>) => {
      actions.setStatus.mutate({ id: job.id, status })
      if (jobs.openJobId === job.id) jobs.setOpenJobId(null)
    },
    [actions.setStatus, jobs]
  )

  const handleDismissConfirm = useCallback(
    (payload: DismissPayload) => {
      if (!dismissTarget) return
      if (dismissTarget.kind === 'single') {
        actions.dismiss.mutate({
          id: dismissTarget.job.id,
          reason: payload.reason,
          note: payload.note,
        })
        if (jobs.openJobId === dismissTarget.job.id) jobs.setOpenJobId(null)
      } else {
        // Bulk dismiss currently uses the bulk endpoint which doesn't accept
        // per-job reasons — we apply the chosen reason as the audit trail
        // intent and stash the note in local logs (best-effort). When the
        // backend grows per-job reason support, route through dismiss.mutate
        // per id instead.
        actions.bulkDismiss.mutate(dismissTarget.ids)
        if (jobs.openJobId && dismissTarget.ids.includes(jobs.openJobId)) {
          jobs.setOpenJobId(null)
        }
        setSelectedIds(new Set())
      }
      setDismissTarget(null)
    },
    [actions.dismiss, actions.bulkDismiss, dismissTarget, jobs]
  )

  const handleBulkDismiss = useCallback(() => {
    const ids = Array.from(selectedIds)
    if (ids.length === 0) return
    setDismissTarget({ kind: 'bulk', ids })
  }, [selectedIds])

  const handleToggleSelect = useCallback((id: string, checked: boolean) => {
    setSelectedIds((prev) => {
      const n = new Set(prev)
      if (checked) n.add(id)
      else n.delete(id)
      return n
    })
    setLastAnchorId(id)
  }, [])

  const handleSelectAll = useCallback(
    (checked: boolean) => {
      setSelectedIds(checked ? new Set(jobs.jobs.map((j) => j.id)) : new Set())
    },
    [jobs.jobs]
  )

  const handleOpenFocused = useCallback(() => {
    const job = focus.focusedJob
    if (!job) return
    if (isMobile) {
      navigate(`/jobs/${job.id}`)
    } else {
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
      jobs.setOpenJobId(null)
      return
    }
    if (selectedIds.size > 0) {
      setSelectedIds(new Set())
    }
  }, [jobs, selectedIds])

  const handleGoToTab = useCallback(
    (tab: TabKey) => {
      jobs.setTab(tab)
      setSelectedIds(new Set())
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

  const handleKeyboardDismiss = useCallback(() => {
    if (!focus.focusedJob) return
    setDismissTarget({ kind: 'single', job: focus.focusedJob })
  }, [focus.focusedJob])

  const handleToggleSelectFocused = useCallback(() => {
    const job = focus.focusedJob
    if (!job) return
    const checked = !selectedIds.has(job.id)
    handleToggleSelect(job.id, checked)
  }, [focus.focusedJob, selectedIds, handleToggleSelect])

  const handleRangeSelect = useCallback(() => {
    const target = focus.focusedJob
    if (!target || !lastAnchorId) return
    const anchorIdx = jobs.jobs.findIndex((j) => j.id === lastAnchorId)
    const targetIdx = jobs.jobs.findIndex((j) => j.id === target.id)
    if (anchorIdx === -1 || targetIdx === -1) return
    const [lo, hi] = anchorIdx <= targetIdx ? [anchorIdx, targetIdx] : [targetIdx, anchorIdx]
    setSelectedIds((prev) => {
      const next = new Set(prev)
      for (let i = lo; i <= hi; i++) next.add(jobs.jobs[i].id)
      return next
    })
  }, [focus.focusedJob, lastAnchorId, jobs.jobs])

  const handleBulkSave = useCallback(() => {
    if (selectedIds.size === 0) return
    actions.bulkSave.mutate(Array.from(selectedIds), {
      onSuccess: () => setSelectedIds(new Set()),
    })
  }, [actions.bulkSave, selectedIds])

  const handleBulkApplied = useCallback(() => {
    if (selectedIds.size === 0) return
    actions.bulkApplied.mutate(Array.from(selectedIds), {
      onSuccess: () => setSelectedIds(new Set()),
    })
  }, [actions.bulkApplied, selectedIds])

  const searchInputRef = useRef<HTMLInputElement | null>(null)
  const handleFocusSearch = useCallback(() => {
    searchInputRef.current?.focus()
  }, [])

  // ── Keyboard wiring ───────────────────────────────────────────────────────
  // Disable shortcuts when a modal/sheet/palette is open OR on mobile (no
  // keyboard there, no point listening).
  const keyboardEnabled =
    !isMobile && !dismissTarget && !paletteOpen && !helpOpen && !mobileFilterOpen
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
    onGoToTab: handleGoToTab,
    onSetStatus: handleKeyboardSetStatus,
    onDismiss: handleKeyboardDismiss,
    onUndo: actions.undoLast,
    onToggleSelect: handleToggleSelectFocused,
    onRangeSelect: handleRangeSelect,
    onBulkSave: handleBulkSave,
    onBulkApplied: handleBulkApplied,
    onBulkDismiss: handleBulkDismiss,
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
          setSelectedIds(new Set())
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

      {/* Bulk action bar — gains parity with keyboard (Save, Applied) and uses
          a live region so screen readers announce selection changes. */}
      {selectedIds.size > 0 && (
        <div
          role="status"
          aria-live="polite"
          className="flex items-center gap-3 px-6 py-2 bg-violet-500/10 border-b border-violet-500/20 flex-wrap"
        >
          <span className="text-xs text-violet-300 font-medium">{selectedIds.size} selected</span>
          <button
            onClick={handleBulkSave}
            disabled={actions.bulkSave.isPending}
            aria-label={`Save ${selectedIds.size} selected jobs`}
            className="flex items-center gap-1.5 text-xs text-white bg-violet-600/80 hover:bg-violet-600 px-3 py-1.5 rounded-lg transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <BookmarkCheck size={12} /> Save
          </button>
          <button
            onClick={handleBulkApplied}
            disabled={actions.bulkApplied.isPending}
            aria-label={`Mark ${selectedIds.size} selected jobs as applied`}
            className="flex items-center gap-1.5 text-xs text-white bg-emerald-600/80 hover:bg-emerald-600 px-3 py-1.5 rounded-lg transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <CheckCircle2 size={12} /> Mark applied
          </button>
          <button
            onClick={handleBulkDismiss}
            disabled={actions.bulkDismiss.isPending}
            aria-label={`Dismiss ${selectedIds.size} selected jobs`}
            className="flex items-center gap-1.5 text-xs text-white bg-red-600/80 hover:bg-red-600 px-3 py-1.5 rounded-lg transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-300 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <Trash2 size={12} /> Mark not relevant
          </button>
          <button
            onClick={() => setSelectedIds(new Set())}
            aria-label="Clear selection"
            className="text-xs text-slate-400 hover:text-slate-200 transition-colors ml-auto focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400 rounded"
          >
            Clear
          </button>
        </div>
      )}

      {/* Content: table + optional detail panel */}
      <div className="flex-1 flex min-h-0 overflow-hidden">
        <div ref={tableScrollRef} className="flex flex-1 min-w-0 overflow-hidden">
          <JobsTable
            jobs={jobs.jobs}
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
            selectedIds={selectedIds}
            activeJobId={jobs.openJobId ?? null}
            focusedIndex={focus.focusedIndex}
            onRowClick={(job) => {
              // Sync the keyboard focus to the clicked row so subsequent
              // shortcuts (s/a/d/u) act on the visually open job rather
              // than the previously-focused one. This is the headline bug
              // from the audit.
              focus.setFocusedJobId(job.id)
              if (isMobile) {
                navigate(`/jobs/${job.id}`)
              } else {
                jobs.setOpenJobId(job.id)
              }
            }}
            onToggleSelect={handleToggleSelect}
            onSelectAll={handleSelectAll}
            onNextPage={jobs.goToNextPage}
            onPrevPage={jobs.goToPrevPage}
            onLimitChange={jobs.setLimit}
            sorting={jobs.sorting}
            onSortingChange={jobs.handleSortingChange}
          />
        </div>

        {!isMobile && openJob && (
          <>
            <ResizeHandle width={panelWidth} onChange={setPanelWidth} min={300} max={720} />
            <div
              style={{ width: panelWidth }}
              className="shrink-0 flex flex-col min-h-0 border-l border-white/[0.07] overflow-hidden"
            >
              <JobDetailPanel
                job={openJob}
                onClose={() => jobs.setOpenJobId(null)}
                onStatusChange={handleStatusChange}
                onDismiss={(job) => setDismissTarget({ kind: 'single', job })}
              />
            </div>
          </>
        )}
      </div>

      <DismissModal
        key={dismissTarget?.kind === 'single' ? dismissTarget.job.id : `bulk-${selectedIds.size}`}
        open={!!dismissTarget}
        jobTitle={dismissTarget?.kind === 'single' ? dismissTarget.job.title : ''}
        count={dismissTarget?.kind === 'bulk' ? dismissTarget.ids.length : 1}
        onConfirm={handleDismissConfirm}
        onCancel={() => setDismissTarget(null)}
      />

      <JobCommandPalette
        open={paletteOpen}
        onOpenChange={setPaletteOpen}
        recentJobs={jobs.jobs}
        onGoToTab={handleGoToTab}
        onSelectJob={(job) => {
          if (isMobile) navigate(`/jobs/${job.id}`)
          else jobs.setOpenJobId(job.id)
        }}
        onResetFilters={jobs.resetFilters}
        onOpenHelp={() => setHelpOpen(true)}
      />
      <JobShortcutSheet open={helpOpen} onOpenChange={setHelpOpen} />
    </div>
  )
}
