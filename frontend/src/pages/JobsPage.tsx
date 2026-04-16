import { useState } from 'react'
import { RefreshCw, Trash2 } from 'lucide-react'
import type { Job } from '@/types'
import { ResizeHandle } from '@/components/ui/resize-handle'
import {
  useJobs,
  useJobActions,
  JobStatusTabs,
  JobFilters,
  JobsTable,
  JobDetailPanel,
  DismissModal,
} from '@/features/jobs'

export function JobsPage() {
  const jobs = useJobs()
  const actions = useJobActions()

  const [activeJob, setActiveJob] = useState<Job | null>(null)
  const [dismissTarget, setDismissTarget] = useState<Job | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [panelWidth, setPanelWidth] = useState(440)

  function handleSave(job: Job) {
    actions.save.mutate({ id: job.id, saved: !job.saved })
  }

  function handleApply(job: Job) {
    actions.apply.mutate(job.id)
    if (activeJob?.id === job.id) setActiveJob(null)
  }

  function handleMarkInterviewing(job: Job) {
    actions.markInterviewing.mutate(job.id)
    if (activeJob?.id === job.id) setActiveJob(null)
  }

  function handleMarkOffer(job: Job) {
    actions.markOffer.mutate(job.id)
    if (activeJob?.id === job.id) setActiveJob(null)
  }

  function handleDismissConfirm(reason: string) {
    if (!dismissTarget) return
    actions.dismiss.mutate({ id: dismissTarget.id, reason })
    if (activeJob?.id === dismissTarget.id) setActiveJob(null)
    setDismissTarget(null)
  }

  function handleBulkDismiss() {
    const ids = Array.from(selectedIds)
    actions.bulkDismiss.mutate(ids)
    if (activeJob && selectedIds.has(activeJob.id)) setActiveJob(null)
    setSelectedIds(new Set())
  }

  const activeFilterCount = [
    jobs.searchQuery.length > 0,
    jobs.selectedCompanies.length > 0,
    jobs.remoteFilter,
    jobs.onSiteFilter,
    JSON.stringify(jobs.expFilter) !== JSON.stringify(['0-2', '3-5']),
    jobs.dateFilter !== 'all',
  ].filter(Boolean).length

  const displayJobs = jobs.searchQuery.trim()
    ? jobs.jobs.filter((j) => {
        const q = jobs.searchQuery.toLowerCase()
        return (
          j.title.toLowerCase().includes(q) ||
          (j.company?.name ?? '').toLowerCase().includes(q) ||
          (j.location ?? '').toLowerCase().includes(q)
        )
      })
    : jobs.jobs

  const displayTotal = jobs.searchQuery.trim() ? displayJobs.length : jobs.total

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-3 border-b border-white/[0.07]">
        <div className="flex items-baseline gap-3">
          <h1 className="text-lg font-semibold text-white">Jobs</h1>
          <span className="text-xs text-slate-400">
            {displayTotal} job{displayTotal !== 1 ? 's' : ''}
            {activeFilterCount > 0 && (
              <>
                {' '}
                ·{' '}
                <span className="text-violet-300">
                  {activeFilterCount} selected filter{activeFilterCount !== 1 ? 's' : ''}
                </span>
              </>
            )}
          </span>
        </div>
        <button
          onClick={() => jobs.refetch()}
          className="p-2 text-slate-400 hover:text-white bg-white/5 border border-white/10 rounded-lg transition-colors"
          title="Refresh"
        >
          <RefreshCw size={14} className={jobs.isLoading ? 'animate-spin' : ''} />
        </button>
      </div>

      <JobStatusTabs
        tab={jobs.tab}
        onChange={(t) => {
          jobs.setTab(t)
          jobs.setPage(1)
        }}
        counts={jobs.counts}
      />

      <JobFilters
        searchQuery={jobs.searchQuery}
        onSearchChange={jobs.setSearchQuery}
        companies={jobs.companies}
        selectedCompanies={jobs.selectedCompanies}
        onCompaniesChange={jobs.setSelectedCompanies}
        remoteFilter={jobs.remoteFilter}
        onRemoteChange={jobs.setRemoteFilter}
        onSiteFilter={jobs.onSiteFilter}
        onOnSiteChange={jobs.setOnSiteFilter}
        expFilter={jobs.expFilter}
        onExpChange={jobs.setExpFilter}
        dateFilter={jobs.dateFilter}
        onDateChange={jobs.setDateFilter}
        onReset={jobs.resetFilters}
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
          jobs={displayJobs}
          total={displayTotal}
          loading={jobs.isLoading}
          tab={jobs.tab}
          page={jobs.page}
          limit={jobs.limit}
          selectedIds={selectedIds}
          activeJobId={activeJob?.id ?? null}
          onRowClick={setActiveJob}
          onToggleSelect={(id, checked) => {
            setSelectedIds((prev) => {
              const n = new Set(prev)
              if (checked) n.add(id)
              else n.delete(id)
              return n
            })
          }}
          onSelectAll={(checked) => {
            setSelectedIds(checked ? new Set(displayJobs.map((j) => j.id)) : new Set())
          }}
          onPageChange={jobs.setPage}
          onLimitChange={(l) => {
            jobs.setLimit(l)
            jobs.setPage(1)
          }}
        />

        {activeJob && (
          <>
            <ResizeHandle width={panelWidth} onChange={setPanelWidth} min={300} max={720} />
            <div
              style={{ width: panelWidth }}
              className="shrink-0 flex flex-col min-h-0 border-l border-white/[0.07] overflow-hidden"
            >
              <JobDetailPanel
                job={activeJob}
                onClose={() => setActiveJob(null)}
                onSave={handleSave}
                onApply={handleApply}
                onMarkInterviewing={handleMarkInterviewing}
                onMarkOffer={handleMarkOffer}
                onDismiss={(job) => setDismissTarget(job)}
              />
            </div>
          </>
        )}
      </div>

      <DismissModal
        open={!!dismissTarget}
        jobTitle={dismissTarget?.title ?? ''}
        onConfirm={handleDismissConfirm}
        onCancel={() => setDismissTarget(null)}
      />
    </div>
  )
}
