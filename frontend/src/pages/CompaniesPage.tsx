import { useEffect, useState, useMemo, useCallback } from 'react'
import { Plus, Search, RefreshCw, Power, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import type { Company } from '@/types'
import { fetchCompanies, deleteCompany, updateCompany, triggerScan, fetchQueue, setAllCompaniesActive } from '@/api/companies'
import { AddCompanyModal } from '@/components/companies/AddCompanyModal'
import { ScanStatsDrawer } from '@/components/companies/ScanStatsDrawer'
import { DataTable } from '@/components/ui/data-table'
import { buildCompanyColumns } from '@/pages/companies-columns'
import { cn } from '@/lib/utils'

export function CompaniesPage() {
  const [companies, setCompanies]   = useState<Company[]>([])
  const [loading, setLoading]       = useState(true)
  const [search, setSearch]         = useState('')
  const [adapterFilter, setAdapterFilter] = useState('all')
  const [activeFilter, setActiveFilter]   = useState<'all' | 'active' | 'inactive'>('all')
  const [addOpen, setAddOpen]       = useState(false)
  const [editCompany, setEditCompany]   = useState<Company | null>(null)
  const [drawerCompany, setDrawerCompany] = useState<Company | null>(null)
  const [panelWidth, setPanelWidth] = useState(400)
  const [scanning, setScanning]     = useState<Set<string>>(new Set())
  const [queueMap, setQueueMap]     = useState<Map<string, 'queued' | 'processing'>>(new Map())
  const [bulkToggling, setBulkToggling] = useState(false)

  // ── Data loading ───────────────────────────────────────────────────────────

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await fetchCompanies()
      setCompanies(data)
    } finally {
      setLoading(false)
    }
  }, [])

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
    const queueTimer   = setInterval(loadQueueStatus, 15_000)
    const companyTimer = setInterval(load, 30_000)
    return () => { clearInterval(queueTimer); clearInterval(companyTimer) }
  }, [load, loadQueueStatus])

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
      setCompanies(cs => cs.filter(c => c.id !== id))
      toast.success('Company deleted')
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
    const allActive = companies.every(c => c.active)
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

  // ── Filtering + columns ────────────────────────────────────────────────────

  const adapters = useMemo(() => {
    const set = new Set(companies.map(c => c.adapter_type))
    return ['all', ...Array.from(set)]
  }, [companies])

  const filtered = useMemo(() => companies.filter(c => {
    if (search && !c.name.toLowerCase().includes(search.toLowerCase())) return false
    if (adapterFilter !== 'all' && c.adapter_type !== adapterFilter) return false
    if (activeFilter === 'active' && !c.active) return false
    if (activeFilter === 'inactive' && c.active) return false
    return true
  }), [companies, search, adapterFilter, activeFilter])

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

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-5 border-b border-white/[0.07]">
        <div>
          <h1 className="text-xl font-semibold text-white">Companies</h1>
          <p className="text-sm text-slate-400 mt-0.5">
            {companies.length} tracked · {companies.filter(c => c.active).length} active
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
        <div className="relative flex-1 min-w-48 max-w-72">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
          <input
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder="Search companies…"
            className="w-full bg-white/5 border border-white/10 rounded-lg pl-8 pr-3 py-2 text-sm text-white placeholder-slate-500 focus:outline-none focus:border-violet-500 transition-colors"
          />
        </div>

        <select
          value={adapterFilter}
          onChange={e => setAdapterFilter(e.target.value)}
          className="bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-slate-300 focus:outline-none focus:border-violet-500 transition-colors"
        >
          {adapters.map(a => (
            <option key={a} value={a}>{a === 'all' ? 'All adapters' : a}</option>
          ))}
        </select>

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

        <button
          onClick={handleBulkToggleActive}
          disabled={bulkToggling || companies.length === 0}
          className="flex items-center gap-1.5 px-3 py-2 text-sm text-slate-400 hover:text-white bg-white/5 border border-white/10 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {bulkToggling
            ? <Loader2 size={14} className="animate-spin" />
            : <Power size={14} />
          }
          {companies.every(c => c.active) ? 'Deactivate All' : 'Activate All'}
        </button>
        <button
          onClick={() => { load(); loadQueueStatus() }}
          className="p-2 text-slate-400 hover:text-white bg-white/5 border border-white/10 rounded-lg transition-colors"
        >
          <RefreshCw size={14} />
        </button>
      </div>

      {/* Table + detail panel */}
      <div className="flex-1 flex min-h-0 overflow-hidden">
        <div className="flex-1 overflow-auto px-6 py-4 min-w-0">
          <DataTable
            columns={columns}
            data={filtered}
            loading={loading}
            emptyMessage={search ? 'No companies match your search' : 'No companies yet — add one to start'}
            getRowId={c => c.id}
            selectedId={drawerCompany?.id}
            onRowClick={setDrawerCompany}
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
