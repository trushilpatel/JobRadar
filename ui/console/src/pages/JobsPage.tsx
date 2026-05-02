import { useState } from 'react'
import { Trash2 } from 'lucide-react'
import type { Job, JobStatus } from '@/types'
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

  function handleStatusChange(job: Job, status: JobStatus) {
    actions.setStatus.mutate({ id: job.id, status })
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
    jobs.expFilter.length > 0,
    jobs.dateFilter !== 'all',
  ].filter(Boolean).length

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <JobStatusTabs
        tab={jobs.tab}
        onChange={(t) => {
          jobs.setTab(t)
          jobs.setPage(1)
        }}
        counts={jobs.counts}
        leftSlot={
          <div className="flex items-baseline gap-3">
            <h1 className="text-lg font-semibold text-white">Jobs</h1>
            <span className="text-xs text-slate-400">
              {jobs.total} job{jobs.total !== 1 ? 's' : ''}
              {activeFilterCount > 0 && (
                <>
                  {' '}
                  ·{' '}
                  <span className="text-violet-300">
                    {activeFilterCount} filter{activeFilterCount !== 1 ? 's' : ''}
                  </span>
                </>
              )}
            </span>
          </div>
        }
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
          jobs={jobs.jobs}
          total={jobs.total}
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
            setSelectedIds(checked ? new Set(jobs.jobs.map((j) => j.id)) : new Set())
          }}
          onPageChange={jobs.setPage}
          onLimitChange={(l) => {
            jobs.setLimit(l)
            jobs.setPage(1)
          }}
          sorting={jobs.sorting}
          onSortingChange={jobs.handleSortingChange}
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
                onStatusChange={handleStatusChange}
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
