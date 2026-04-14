import { useEffect, useState, useMemo, useCallback, useRef } from 'react'
import { Plus, Search, RefreshCw, Power, Loader2, X } from 'lucide-react'
import { toast } from 'sonner'
import type { Company } from '@/types'
import {
  fetchCompanies, deleteCompany, updateCompany, triggerScan, fetchQueue,
  setAllCompaniesActive, type CompanyFilters,
} from '@/api/companies'
import { AddCompanyModal } from '@/components/companies/AddCompanyModal'
import { ScanStatsDrawer } from '@/components/companies/ScanStatsDrawer'
import { DataTable } from '@/components/ui/data-table'
import { buildCompanyColumns } from '@/pages/companies-columns'
import { cn } from '@/lib/utils'

// Known adapter types — hardcoded to avoid needing all companies in memory
const ADAPTER_OPTIONS = [
  'greenhouse', 'lever', 'ashby',
  'google', 'netflix', 'uber', 'apple', 'meta', 'microsoft', 'rippling',
]

export function CompaniesPage() {
  // ── Pagination + server state ──────────────────────────────────────────────
  const [companies, setCompanies]     = useState<Company[]>([])
  const [total, setTotal]             = useState(0)
  const [totalActive, setTotalActive] = useState(0)
  const [page, setPage]               = useState(1)
  const [limit, setLimit]             = useState(20)
  const [loading, setLoading]         = useState(true)

  // ── Filters ────────────────────────────────────────────────────────────────
  const [search, setSearch]               = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [adapterFilter, setAdapterFilter] = useState('')
  const [activeFilter, setActiveFilter]   = useState<'all' | 'active' | 'inactive'>('all')

  // Debounce search input (300 ms)
  const debounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  function handleSearchChange(val: string) {
    setSearch(val)
    if (debounceTimer.current) clearTimeout(debounceTimer.current)
    debounceTimer.current = setTimeout(() => setDebouncedSearch(val), 300)
  }

  const filters = useMemo<CompanyFilters>(() => ({
    search: debouncedSearch || undefined,
    adapterType: adapterFilter || undefined,
    active: activeFilter === 'active' ? true : activeFilter === 'inactive' ? false : undefined,
  }), [debouncedSearch, adapterFilter, activeFilter])

  // Reset page whenever filters change
  useEffect(() => { setPage(1) }, [filters])

  // ── Drawer / modal state ──────────────────────────────────────────────────
  const [addOpen, setAddOpen]         = useState(false)
  const [editCompany, setEditCompany] = useState<Company | null>(null)
  const [drawerCompany, setDrawerCompany] = useState<Company | null>(null)
  const [panelWidth, setPanelWidth]   = useState(400)
  const [scanning, setScanning]       = useState<Set<string>>(new Set())
  const [queueMap, setQueueMap]       = useState<Map<string, 'queued' | 'processing'>>(new Map())
  const [bulkToggling, setBulkToggling] = useState(false)

  // ── Data loading ───────────────────────────────────────────────────────────

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await fetchCompanies(filters, page, limit)
      setCompanies(res.data ?? [])
      setTotal(res.total ?? 0)
      setTotalActive(res.total_active ?? 0)
    } catch {
      setCompanies([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }, [filters, page, limit])

  const loadQueueStatus = useCallback(async () => {
    try {
      const entries = await fetchQueue()
      const map = new Map<string, 'queued' | 'processing'>()
      for (const e of entries) map.set(e.company_id, e.status)
      setQueueMap(map)
    } catch {
      // non-fatal
    }
  }, [])

  useEffect(() => {
    load()
    loadQueueStatus()
  }, [load, loadQueueStatus])

  useEffect(() => {
    const queueTimer   = setInterval(loadQueueStatus, 15_000)
    const companyTimer = setInterval(load, 30_000)
    return () => { clearInterval(queueTimer); clearInterval(companyTimer) }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // ── Drag-resize panel ──────────────────────────────────────────────────────

  const startDrag = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    const startX = e.clientX
    const startWidth = panelWidth
    const onMove = (e: MouseEvent) =>
      setPanelWidth(Math.min(Math.max(startWidth + (startX - e.clientX), 280), 680))
    const onUp = () => {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
  }, [panelWidth])

  // ── Row actions ────────────────────────────────────────────────────────────

  async function handleToggleActive(company: Company) {
    const next = !company.active
    setCompanies(cs => cs.map(c => c.id === company.id ? { ...c, active: next } : c))
    try {
      await updateCompany(company.id, { active: next })
    } catch {
      setCompanies(cs => cs.map(c => c.id === company.id ? { ...c, active: !next } : c))
    }
  }

  async function handleDelete(id: string) {
    if (!confirm('Delete this company? This cannot be undone.')) return
    try {
      await deleteCompany(id)
      toast.success('Company deleted')
      load()
    } catch {
      toast.error('Failed to delete company')
    }
  }

  async function handleScan(company: Company) {
    setScanning(s => new Set(s).add(company.id))
    try {
      await triggerScan(company.id)
      toast.success(`Scan queued for ${company.name}`)
      loadQueueStatus()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to queue scan')
    } finally {
      setScanning(s => { const n = new Set(s); n.delete(company.id); return n })
    }
  }

  async function handleBulkToggleActive() {
    const allActive = totalActive > 0 && totalActive === total
    const next = !allActive
    setBulkToggling(true)
    try {
      await setAllCompaniesActive(next)
      await load()
      toast.success(next ? 'All companies activated' : 'All companies deactivated')
    } catch {
      toast.error('Failed to update companies')
    } finally {
      setBulkToggling(false)
    }
  }

  // ── Columns ────────────────────────────────────────────────────────────────

  const columns = useMemo(() => buildCompanyColumns({
    onScan: handleScan,
    onEdit: setEditCompany,
    onDelete: handleDelete,
    onToggleActive: handleToggleActive,
    scanning,
    queueMap,
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }), [scanning, queueMap])

  // ── Render ─────────────────────────────────────────────────────────────────

  const allActive = totalActive > 0 && totalActive === total

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-5 border-b border-white/[0.07]">
        <div>
          <h1 className="text-xl font-semibold text-white">Companies</h1>
          <p className="text-sm text-slate-400 mt-0.5">
            {total} tracked · {totalActive} active
          </p>
        </div>
        <button
          onClick={() => setAddOpen(true)}
          className="flex items-center gap-2 bg-violet-600 hover:bg-violet-500 text-white text-sm font-medium px-4 py-2 rounded-lg transition-colors"
        >
          <Plus size={15} /> Add Company
        </button>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3 px-6 py-3 border-b border-white/[0.07] flex-wrap">
        {/* Search */}
        <div className="relative flex-1 min-w-48 max-w-72">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
          <input
            value={search}
            onChange={e => handleSearchChange(e.target.value)}
            placeholder="Search companies…"
            className="w-full bg-white/5 border border-white/10 rounded-lg pl-8 pr-8 py-2 text-sm text-white placeholder-slate-500 focus:outline-none focus:border-violet-500 transition-colors"
          />
          {search && (
            <button
              onClick={() => { setSearch(''); setDebouncedSearch('') }}
              className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300"
            >
              <X size={13} />
            </button>
          )}
        </div>

        {/* Adapter filter */}
        <select
          value={adapterFilter}
          onChange={e => setAdapterFilter(e.target.value)}
          className="bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-slate-300 focus:outline-none focus:border-violet-500 transition-colors"
        >
          <option value="">All adapters</option>
          {ADAPTER_OPTIONS.map(a => (
            <option key={a} value={a}>{a}</option>
          ))}
        </select>

        {/* Active filter */}
        <div className="flex rounded-lg border border-white/10 overflow-hidden">
          {(['all', 'active', 'inactive'] as const).map(f => (
            <button
              key={f}
              onClick={() => setActiveFilter(f)}
              className={cn(
                'px-3 py-2 text-sm transition-colors capitalize',
                activeFilter === f ? 'bg-violet-600 text-white' : 'text-slate-400 hover:text-white bg-white/5'
              )}
            >
              {f}
            </button>
          ))}
        </div>

        {/* Bulk toggle */}
        <button
          onClick={handleBulkToggleActive}
          disabled={bulkToggling || total === 0}
          className="flex items-center gap-1.5 px-3 py-2 text-sm text-slate-400 hover:text-white bg-white/5 border border-white/10 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {bulkToggling
            ? <Loader2 size={14} className="animate-spin" />
            : <Power size={14} />
          }
          {allActive ? 'Deactivate All' : 'Activate All'}
        </button>

        <button
          onClick={() => { load(); loadQueueStatus() }}
          className="p-2 text-slate-400 hover:text-white bg-white/5 border border-white/10 rounded-lg transition-colors"
        >
          <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
        </button>
      </div>

      {/* Table + detail panel */}
      <div className="flex-1 flex min-h-0 overflow-hidden">
        <div className="flex-1 overflow-auto px-6 py-4 min-w-0">
          <DataTable
            columns={columns}
            data={companies}
            loading={loading}
            emptyMessage={debouncedSearch ? 'No companies match your search' : 'No companies yet — add one to start'}
            getRowId={c => c.id}
            selectedId={drawerCompany?.id}
            onRowClick={setDrawerCompany}
            serverPagination={{
              page,
              limit,
              total,
              onPageChange: setPage,
              onLimitChange: (l) => { setLimit(l); setPage(1) },
            }}
          />
        </div>

        {drawerCompany && (
          <>
            <div
              onMouseDown={startDrag}
              className="w-1 shrink-0 cursor-col-resize bg-white/[0.06] hover:bg-violet-500/50 transition-colors"
            />
            <div style={{ width: panelWidth }} className="shrink-0 flex flex-col min-h-0 border-l border-white/[0.07] overflow-hidden">
              <ScanStatsDrawer company={drawerCompany} onClose={() => setDrawerCompany(null)} />
            </div>
          </>
        )}
      </div>

      <AddCompanyModal
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onCreated={async c => {
          load()
          if (c.active) {
            try {
              await triggerScan(c.id)
              toast.success(`${c.name} added and scan queued`)
              loadQueueStatus()
            } catch {
              toast.success(`${c.name} added — click Scan to start scanning`)
            }
          } else {
            toast.success(`${c.name} added`)
          }
        }}
      />
      <AddCompanyModal
        open={!!editCompany}
        initialData={editCompany ?? undefined}
        onClose={() => setEditCompany(null)}
        onUpdated={updated => {
          load()
          toast.success(`${updated.name} updated`)
        }}
      />
    </div>
  )
}
