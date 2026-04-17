import { useState } from 'react'
import { toast } from 'sonner'
import type { Company } from '@/types'
import { triggerScan } from '@/api/companies'
import { ADAPTER_TYPES } from '@/types'
import { ResizeHandle } from '@/components/ui/resize-handle'
import {
  useCompanies,
  useCompanyActions,
  CompaniesHeader,
  PipelineStrip,
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
  const [panelWidth, setPanelWidth] = useState(420)
  const [adapterOptions, setAdapterOptions] = useState<string[]>([...ADAPTER_TYPES])

  const bulkAllActive = c.totalActive > 0 && c.totalActive === c.total

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <CompaniesHeader
        total={c.total}
        totalActive={c.totalActive}
        onAddClick={() => setAddOpen(true)}
      />

      <PipelineStrip
        errorCount={c.counts.errors}
        errorFilterActive={c.statusFilter === 'error'}
        onErrorToggle={() => c.setStatusFilter(c.statusFilter === 'error' ? 'none' : 'error')}
      />

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
        statusFilter={c.statusFilter}
        onStatusFilterChange={c.setStatusFilter}
        bulkToggling={actions.bulkToggling}
        bulkAllActive={bulkAllActive}
        onBulkToggle={() => actions.bulkToggleActive(bulkAllActive)}
        onReset={c.resetFilters}
      />

      <div className="flex-1 flex min-h-0 overflow-hidden">
        <CompaniesTable
          companies={c.companies}
          total={c.statusFilter !== 'none' ? c.companies.length : c.total}
          loading={c.loading}
          page={c.page}
          limit={c.limit}
          onPageChange={c.setPage}
          onLimitChange={(l) => {
            c.setLimit(l)
            c.setPage(1)
          }}
          emptyMessage={
            c.statusFilter !== 'none'
              ? `No ${c.statusFilter} companies on this page — try a different page or reset filters`
              : c.search
                ? 'No companies match your search'
                : 'No companies match the current filters'
          }
          activeCompanyId={drawerCompany?.id}
          onRowClick={setDrawerCompany}
          actions={{
            onScan: actions.scan,
            onEdit: setEditCompany,
            onDelete: actions.remove,
            onToggleActive: actions.toggleActive,
            scanning: actions.scanning,
            queueMap: c.queueMap,
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
                queueStatus={c.queueMap.get(drawerCompany.id)}
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
        onCreated={async (company) => {
          c.refetch()
          if (company.active) {
            try {
              await triggerScan(company.id)
              toast.success(`${company.name} added and scan queued`)
              c.refetch()
            } catch {
              toast.success(`${company.name} added — click Scan to start scanning`)
            }
          } else {
            toast.success(`${company.name} added`)
          }
        }}
      />

      <AddCompanyModal
        open={!!editCompany}
        initialData={editCompany ?? undefined}
        onClose={() => setEditCompany(null)}
        onUpdated={(updated) => {
          c.refetch()
          toast.success(`${updated.name} updated`)
        }}
      />
    </div>
  )
}
