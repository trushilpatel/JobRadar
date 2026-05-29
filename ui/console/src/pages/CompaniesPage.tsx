import { useState, useCallback, type CSSProperties } from 'react'
import { useNavigate } from 'react-router-dom'
import type { Company } from '@/types'
import { ADAPTER_TYPES } from '@/types'
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
import { EMPTY_MESSAGES } from '@/features/companies'
import { jobsUrl } from '@/features/jobs'
import { ResizeHandle } from '@/components/ui/resize-handle'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  useCompanies,
  useCompanyActions,
  CompaniesHeader,
  CompaniesFilterBar,
  CompaniesTable,
  CompanyDetailDrawer,
  AddCompanyModal,
} from '@/features/companies'

export function CompaniesPage() {
  const c = useCompanies()
  const actions = useCompanyActions()

  const [addOpen, setAddOpen] = useState(false)
  const [editCompany, setEditCompany] = useState<Company | null>(null)
  const [drawerCompany, setDrawerCompany] = useState<Company | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Company | null>(null)
  const [panelWidth, setPanelWidth] = useState(480)
  const navigate = useNavigate()
  const handleViewJobs = useCallback(
    (companyName: string) => navigate(jobsUrl({ companies: [companyName] })),
    [navigate]
  )
  const [adapterOptions, setAdapterOptions] = useState<string[]>([...ADAPTER_TYPES])

  const bulkAllActive = c.companies.length > 0 && c.companies.every((co) => co.active)

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <CompaniesHeader autoRefresh={c.autoRefresh} onToggleAutoRefresh={c.toggleAutoRefresh} />

      <CompaniesFilterBar
        search={c.search}
        onSearchChange={c.handleSearchChange}
        adapters={c.adapters}
        onAdaptersChange={c.setAdapters}
        adapterOptions={adapterOptions}
        onAddAdapter={(v) => {
          if (!adapterOptions.includes(v)) setAdapterOptions((opts) => [...opts, v])
        }}
        activeFilter={c.activeFilter}
        onActiveFilterChange={c.setActiveFilter}
        scanStatus={c.scanStatus}
        onScanStatusChange={c.setScanStatus}
        bulkToggling={actions.bulkToggling}
        bulkAllActive={bulkAllActive}
        onBulkToggle={() => actions.bulkToggleActive(bulkAllActive)}
        runningAll={actions.runningAll}
        onRunAll={actions.scanAll}
        onReset={c.resetFilters}
        hasActiveFilters={c.hasActiveFilters}
        onAddClick={() => setAddOpen(true)}
      />

      <ErrorBoundary>
        <div className="flex-1 flex min-h-0 overflow-hidden">
          <CompaniesTable
            companies={c.companies}
            page={c.page}
            itemsOnPage={c.itemsOnPage}
            itemsBeforePage={c.itemsBeforePage}
            knownTotal={c.total}
            loading={c.isLoading || c.isFetching}
            limit={c.limit}
            onLimitChange={c.setLimit}
            nextCursor={c.nextCursor}
            hasPrev={c.hasPrev}
            onNext={c.goToNextPage}
            onPrev={c.goToPrevPage}
            onFirst={c.goToFirstPage}
            emptyMessage={
              c.hasActiveFilters ? EMPTY_MESSAGES.WITH_FILTERS : EMPTY_MESSAGES.WITHOUT_FILTERS
            }
            activeCompanyId={drawerCompany?.id}
            onRowClick={setDrawerCompany}
            sorting={c.sorting}
            onSortingChange={c.handleSortingChange}
            actions={{
              onScan: actions.scan,
              onEdit: setEditCompany,
              onDelete: (id) => setDeleteTarget(c.companies.find((co) => co.id === id) ?? null),
              onToggleActive: actions.toggleActive,
              scanning: actions.scanning,
            }}
          />

          {drawerCompany && (
            <>
              <ResizeHandle width={panelWidth} onChange={setPanelWidth} />
              <aside
                style={{ ['--panel-width' as string]: `${panelWidth}px` } as CSSProperties}
                className="shrink-0 min-h-0 border-l border-white/[0.07] overflow-hidden [width:var(--panel-width)]"
              >
                <CompanyDetailDrawer
                  company={drawerCompany}
                  onClose={() => setDrawerCompany(null)}
                  onScan={actions.scan}
                  onEdit={setEditCompany}
                  onDelete={(id) => {
                    actions.remove(id)
                    setDrawerCompany(null)
                  }}
                  onViewJobs={handleViewJobs}
                />
              </aside>
            </>
          )}
        </div>
      </ErrorBoundary>

      <AddCompanyModal
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onCreated={(company) => {
          setAddOpen(false)
          actions.handleCompanyAdded(company)
        }}
      />

      <AddCompanyModal
        open={!!editCompany}
        initialData={editCompany ?? undefined}
        onClose={() => setEditCompany(null)}
        onUpdated={(updated) => {
          setEditCompany(null)
          actions.handleCompanyUpdated(updated)
        }}
      />

      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent className="bg-[#0d0d14] border-white/10">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-white">Delete Company</AlertDialogTitle>
            <AlertDialogDescription>
              This will permanently delete <strong>{deleteTarget?.name}</strong> and all its scan
              history. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-red-600 hover:bg-red-500 text-white"
              onClick={() => {
                if (deleteTarget) {
                  actions.remove(deleteTarget.id)
                  if (drawerCompany?.id === deleteTarget.id) setDrawerCompany(null)
                  setDeleteTarget(null)
                }
              }}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
