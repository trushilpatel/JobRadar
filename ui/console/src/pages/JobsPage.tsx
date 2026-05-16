import { useCallback, useMemo, useRef, useState } from 'react'
import { Trash2, Keyboard } from 'lucide-react'
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
import { useIsMobile } from '@/lib/use-media-query'

type SettableStatus = Exclude<JobStatus, 'dismissed' | 'new'>

export function JobsPage() {
  const jobs = useJobs()
  const actions = useJobActions()
  const navigate = useNavigate()
  const isMobile = useIsMobile()

  // Local UI state — selection, focused row, layout widths, overlay flags.
  // None of these survive a refresh by design.
  const [dismissTarget, setDismissTarget] = useState<Job | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [panelWidth, setPanelWidth] = useState(440)
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [helpOpen, setHelpOpen] = useState(false)
  const [mobileFilterOpen, setMobileFilterOpen] = useState(false)
  // Anchor for shift-range selection (last toggled row id).
  const [lastAnchorId, setLastAnchorId] = useState<string | null>(null)

  // Focused-row index — separate from the URL-driven open preview.
  const focus = useFocusedJob(jobs.jobs)

  // The preview job is URL-driven. Look it up against the current page.
  const openJob = useMemo(
    () => jobs.jobs.find((j) => j.id === jobs.openJobId) ?? null,
    [jobs.jobs, jobs.openJobId]
  )

  // ── Action handlers ───────────────────────────────────────────────────────
  const handleStatusChange = useCallback(
    (job: Job, status: Exclude<JobStatus, 'dismissed'>) => {
      actions.setStatus.mutate({ id: job.id, status })
      if (jobs.openJobId === job.id) jobs.setOpenJobId(null)
    },
    [actions.setStatus, jobs]
  )

  const handleDismissConfirm = useCallback(
    (reason: string) => {
      if (!dismissTarget) return
      actions.dismiss.mutate({ id: dismissTarget.id, reason })
      if (jobs.openJobId === dismissTarget.id) jobs.setOpenJobId(null)
      setDismissTarget(null)
    },
    [actions.dismiss, dismissTarget, jobs]
  )

  const handleBulkDismiss = useCallback(() => {
    const ids = Array.from(selectedIds)
    if (ids.length === 0) return
    actions.bulkDismiss.mutate(ids)
    if (jobs.openJobId && selectedIds.has(jobs.openJobId)) jobs.setOpenJobId(null)
    setSelectedIds(new Set())
  }, [actions.bulkDismiss, selectedIds, jobs])

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
    setDismissTarget(focus.focusedJob)
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

  const handleSelectAllOnPage = useCallback(() => {
    setSelectedIds(new Set(jobs.jobs.map((j) => j.id)))
  }, [jobs.jobs])

  const handleBulkSave = useCallback(() => {
    if (selectedIds.size === 0) return
    actions.bulkSave.mutate(Array.from(selectedIds))
  }, [actions.bulkSave, selectedIds])

  const handleBulkApplied = useCallback(() => {
    if (selectedIds.size === 0) return
    actions.bulkApplied.mutate(Array.from(selectedIds))
  }, [actions.bulkApplied, selectedIds])

  const searchInputRef = useRef<HTMLInputElement | null>(null)
  const handleFocusSearch = useCallback(() => {
    searchInputRef.current?.focus()
  }, [])

  // ── Keyboard wiring ───────────────────────────────────────────────────────
  // Disable shortcuts when a modal/sheet/palette is open so their own keys
  // (esc to close, etc.) take over.
  const keyboardEnabled = !dismissTarget && !paletteOpen && !helpOpen && !mobileFilterOpen
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
    onSelectAll: handleSelectAllOnPage,
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

      {/* Bulk action bar */}
      {selectedIds.size > 0 && (
        <div className="flex items-center gap-3 px-6 py-2 bg-violet-500/10 border-b border-violet-500/20">
          <span className="text-xs text-violet-300 font-medium">{selectedIds.size} selected</span>
          <button
            onClick={handleBulkDismiss}
            className="flex items-center gap-1.5 text-xs text-white bg-red-600/80 hover:bg-red-600 px-3 py-1.5 rounded-lg transition-colors"
          >
            <Trash2 size={12} /> Mark not relevant
          </button>
          <button
            onClick={() => setSelectedIds(new Set())}
            className="text-xs text-slate-400 hover:text-slate-200 transition-colors ml-auto"
          >
            Clear
          </button>
        </div>
      )}

      {/* Content: table + optional detail panel */}
      <div className="flex-1 flex min-h-0 overflow-hidden">
        <JobsTable
          jobs={jobs.jobs}
          loading={jobs.isLoading}
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
                onDismiss={(job) => setDismissTarget(job)}
              />
            </div>
          </>
        )}
      </div>

      <DismissModal
        key={dismissTarget?.id ?? ''}
        open={!!dismissTarget}
        jobTitle={dismissTarget?.title ?? ''}
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
