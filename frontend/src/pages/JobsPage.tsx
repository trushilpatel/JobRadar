import { useState } from 'react'
import { RefreshCw, Trash2 } from 'lucide-react'
import type { Job } from '@/types'
import {
  useJobs, useJobActions,
  JobStatusTabs, JobFilters, JobsTable,
  JobDetailPanel, DismissModal,
} from '@/features/jobs'

export function JobsPage() {
  const jobs    = useJobs()
  const actions = useJobActions()

  const [activeJob, setActiveJob]         = useState<Job | null>(null)
  const [dismissTarget, setDismissTarget] = useState<Job | null>(null)
  const [selectedIds, setSelectedIds]     = useState<Set<string>>(new Set())
  const [panelWidth, setPanelWidth]       = useState(440)

  function startDrag(e: React.MouseEvent) {
    e.preventDefault()
    const startX     = e.clientX
    const startWidth = panelWidth
    function onMove(e: MouseEvent) {
      setPanelWidth(Math.min(Math.max(startWidth + (startX - e.clientX), 300), 720))
    }
    function onUp() {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
  }

  function handleSave(job: Job) {
    actions.save.mutate({ id: job.id, saved: !job.saved })
  }

  function handleApply(job: Job) {
    actions.apply.mutate(job.id)
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

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-4 border-b border-white/[0.07]">
        <div>
          <h1 className="text-xl font-semibold text-white">Jobs</h1>
          <p className="text-sm text-slate-400 mt-0.5">
            {jobs.total} job{jobs.total !== 1 ? 's' : ''}
          </p>
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
        total={jobs.total}
        onChange={t => { jobs.setTab(t); jobs.setPage(1) }}
      />

      <JobFilters
        companies={jobs.companies}
        selectedCompanies={jobs.selectedCompanies}
        onCompaniesChange={jobs.setSelectedCompanies}
        remoteFilter={jobs.remoteFilter}
        onRemoteChange={jobs.setRemoteFilter}
        expFilter={jobs.expFilter}
        onExpChange={jobs.setExpFilter}
        dateFilter={jobs.dateFilter}
        onDateChange={jobs.setDateFilter}
        sponsorshipFilter={jobs.sponsorshipFilter}
        onSponsorshipChange={jobs.setSponsorshipFilter}
        noSponsorshipFilter={jobs.noSponsorshipFilter}
        onNoSponsorshipChange={jobs.setNoSponsorshipFilter}
        noRequirementsFilter={jobs.noRequirementsFilter}
        onNoRequirementsChange={jobs.setNoRequirementsFilter}
        citizenshipReqFilter={jobs.citizenshipReqFilter}
        onCitizenshipReqChange={jobs.setCitizenshipReqFilter}
        employTypes={jobs.employTypes}
        onEmployTypesChange={jobs.setEmployTypes}
      />

      {/* Bulk action bar */}
      {selectedIds.size > 0 && (
        <div className="flex items-center gap-3 px-6 py-2 bg-violet-500/10 border-b border-violet-500/20">
          <span className="text-xs text-violet-300 font-medium">
            {selectedIds.size} selected
          </span>
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
            setSelectedIds(prev => {
              const n = new Set(prev)
              checked ? n.add(id) : n.delete(id)
              return n
            })
          }}
          onSelectAll={checked => {
            setSelectedIds(checked ? new Set(jobs.jobs.map(j => j.id)) : new Set())
          }}
          onPageChange={jobs.setPage}
          onLimitChange={l => { jobs.setLimit(l); jobs.setPage(1) }}
        />

        {activeJob && (
          <>
            <div
              onMouseDown={startDrag}
              className="w-1 shrink-0 cursor-col-resize bg-white/[0.06] hover:bg-violet-500/50 transition-colors"
            />
            <div
              style={{ width: panelWidth }}
              className="shrink-0 flex flex-col min-h-0 border-l border-white/[0.07] overflow-hidden"
            >
              <JobDetailPanel
                job={activeJob}
                onClose={() => setActiveJob(null)}
                onSave={handleSave}
                onApply={handleApply}
                onDismiss={job => setDismissTarget(job)}
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
