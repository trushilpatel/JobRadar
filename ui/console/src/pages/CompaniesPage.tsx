import { useState } from 'react'
import type { Company } from '@/types'
import { ADAPTER_TYPES } from '@/types'
import { EMPTY_MESSAGES } from '@/features/companies/constants'
import { ResizeHandle } from '@/components/ui/resize-handle'
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
  const [panelWidth, setPanelWidth] = useState(480)
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

      <div className="flex-1 flex min-h-0 overflow-hidden">
        <CompaniesTable
          companies={c.companies}
          total={c.total}
          loading={c.isLoading || c.isFetching}
          page={c.page}
          limit={c.limit}
          onPageChange={c.setPage}
          onLimitChange={c.setLimit}
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
            onDelete: actions.remove,
            onToggleActive: actions.toggleActive,
            scanning: actions.scanning,
          }}
        />

        {drawerCompany && (
          <>
            <ResizeHandle width={panelWidth} onChange={setPanelWidth} />
            <aside
              style={{ width: panelWidth }}
              className="shrink-0 min-h-0 border-l border-white/[0.07] overflow-hidden"
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
              />
            </aside>
          </>
        )}
      </div>

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
    </div>
  )
}
