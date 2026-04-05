import { useEffect, useState, useMemo } from 'react'
import {
  Plus, Search, RefreshCw, Trash2, Pencil, ChevronUp, ChevronDown,
  CheckCircle2, XCircle, Clock, Minus, ExternalLink
} from 'lucide-react'
import type { Company } from '@/types'
import { fetchCompanies, deleteCompany, updateCompany, triggerScan } from '@/api/companies'
import { AddCompanyModal } from '@/components/companies/AddCompanyModal'
import { ScanStatsDrawer } from '@/components/companies/ScanStatsDrawer'
import { formatRelativeTime, cn } from '@/lib/utils'

type SortKey = 'name' | 'last_scan_at' | 'last_scan_status'
type SortDir = 'asc' | 'desc'

export function CompaniesPage() {
  const [companies, setCompanies] = useState<Company[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [adapterFilter, setAdapterFilter] = useState('all')
  const [activeFilter, setActiveFilter] = useState<'all' | 'active' | 'inactive'>('all')
  const [sort, setSort] = useState<{ key: SortKey; dir: SortDir }>({ key: 'name', dir: 'asc' })
  const [addOpen, setAddOpen] = useState(false)
  const [editCompany, setEditCompany] = useState<Company | null>(null)
  const [drawerCompany, setDrawerCompany] = useState<Company | null>(null)
  const [scanning, setScanning] = useState<Set<string>>(new Set())
  const [toastMsg, setToastMsg] = useState('')

  useEffect(() => {
    load()
  }, [])

  async function load() {
    setLoading(true)
    try {
      const data = await fetchCompanies()
      setCompanies(data)
    } finally {
      setLoading(false)
    }
  }

  function toast(msg: string) {
    setToastMsg(msg)
    setTimeout(() => setToastMsg(''), 3000)
  }

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
      toast('Company deleted')
    } catch {
      toast('Failed to delete company')
    }
  }

  async function handleScan(company: Company) {
    setScanning(s => new Set(s).add(company.id))
    try {
      await triggerScan(company.id)
      toast(`Scan queued for ${company.name}`)
    } catch {
      toast('Failed to queue scan')
    } finally {
      setScanning(s => { const n = new Set(s); n.delete(company.id); return n })
    }
  }

  function toggleSort(key: SortKey) {
    setSort(s => s.key === key ? { key, dir: s.dir === 'asc' ? 'desc' : 'asc' } : { key, dir: 'asc' })
  }

  const adapters = useMemo(() => {
    const set = new Set(companies.map(c => c.adapter_type))
    return ['all', ...Array.from(set)]
  }, [companies])

  const filtered = useMemo(() => {
    return companies
      .filter(c => {
        if (search && !c.name.toLowerCase().includes(search.toLowerCase())) return false
        if (adapterFilter !== 'all' && c.adapter_type !== adapterFilter) return false
        if (activeFilter === 'active' && !c.active) return false
        if (activeFilter === 'inactive' && c.active) return false
        return true
      })
      .sort((a, b) => {
        let av: string = '', bv: string = ''
        if (sort.key === 'name') { av = a.name; bv = b.name }
        else if (sort.key === 'last_scan_at') { av = a.last_scan_at ?? ''; bv = b.last_scan_at ?? '' }
        else if (sort.key === 'last_scan_status') { av = a.last_scan_status ?? ''; bv = b.last_scan_status ?? '' }
        const cmp = av.localeCompare(bv)
        return sort.dir === 'asc' ? cmp : -cmp
      })
  }, [companies, search, adapterFilter, activeFilter, sort])

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-5 border-b border-white/[0.07]">
        <div>
          <h1 className="text-xl font-semibold text-white">Companies</h1>
          <p className="text-sm text-slate-400 mt-0.5">{companies.length} tracked · {companies.filter(c => c.active).length} active</p>
        </div>
        <button
          onClick={() => setAddOpen(true)}
          className="flex items-center gap-2 bg-violet-600 hover:bg-violet-500 text-white text-sm font-medium px-4 py-2 rounded-lg transition-colors"
        >
          <Plus size={15} />
          Add Company
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
                activeFilter === f
                  ? 'bg-violet-600 text-white'
                  : 'text-slate-400 hover:text-white bg-white/5'
              )}
            >
              {f}
            </button>
          ))}
        </div>

        <button onClick={load} className="p-2 text-slate-400 hover:text-white bg-white/5 border border-white/10 rounded-lg transition-colors">
          <RefreshCw size={14} />
        </button>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto px-6 py-4">
        {loading ? (
          <div className="flex items-center justify-center h-48">
            <div className="w-6 h-6 border-2 border-violet-500 border-t-transparent rounded-full animate-spin" />
          </div>
        ) : filtered.length === 0 ? (
          <div className="text-center py-20 text-slate-500">
            {search ? 'No companies match your search' : 'No companies yet — add one to start'}
          </div>
        ) : (
          <table className="w-full border-collapse">
            <thead>
              <tr className="text-left">
                {[
                  { key: 'name', label: 'Company' },
                  { key: null, label: 'Adapter' },
                  { key: 'last_scan_status', label: 'Status' },
                  { key: 'last_scan_at', label: 'Last Scan' },
                  { key: null, label: 'Active' },
                  { key: null, label: '' },
                ].map(({ key, label }, i) => (
                  <th
                    key={i}
                    onClick={() => key && toggleSort(key as SortKey)}
                    className={cn(
                      'pb-3 text-xs font-medium text-slate-400 uppercase tracking-wider pr-4',
                      key && 'cursor-pointer hover:text-white select-none'
                    )}
                  >
                    <span className="inline-flex items-center gap-1">
                      {label}
                      {key && sort.key === key && (
                        sort.dir === 'asc' ? <ChevronUp size={12} /> : <ChevronDown size={12} />
                      )}
                    </span>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.04]">
              {filtered.map(company => (
                <tr key={company.id} className="group hover:bg-white/[0.02] transition-colors">
                  <td className="py-3 pr-4">
                    <div
                      className="cursor-pointer"
                      onClick={() => setDrawerCompany(company)}
                    >
                      <p className="text-sm font-medium text-white group-hover:text-violet-300 transition-colors">
                        {company.name}
                      </p>
                      <a
                        href={company.url}
                        target="_blank"
                        rel="noreferrer"
                        onClick={e => e.stopPropagation()}
                        className="flex items-center gap-1 text-xs text-slate-500 hover:text-slate-300 transition-colors mt-0.5 w-fit"
                      >
                        {(() => { try { return new URL(company.url).hostname } catch { return company.url } })()}
                        <ExternalLink size={10} />
                      </a>
                    </div>
                  </td>
                  <td className="py-3 pr-4">
                    <span className="text-xs bg-white/[0.07] text-slate-300 px-2 py-0.5 rounded-md font-mono">
                      {company.adapter_type}
                    </span>
                  </td>
                  <td className="py-3 pr-4">
                    <ScanStatusBadge status={company.last_scan_status} />
                  </td>
                  <td className="py-3 pr-4 text-sm text-slate-400">
                    {formatRelativeTime(company.last_scan_at)}
                  </td>
                  <td className="py-3 pr-4">
                    <button
                      onClick={() => handleToggleActive(company)}
                      className={cn(
                        'relative inline-flex w-10 h-6 rounded-full transition-colors duration-200 focus:outline-none shrink-0',
                        company.active ? 'bg-violet-600' : 'bg-white/15'
                      )}
                    >
                      <span className={cn(
                        'absolute top-1 w-4 h-4 bg-white rounded-full shadow transition-transform duration-200',
                        company.active ? 'translate-x-5' : 'translate-x-1'
                      )} />
                    </button>
                  </td>
                  <td className="py-3">
                    <div className="flex items-center gap-1 justify-end">
                      <button
                        onClick={() => handleScan(company)}
                        disabled={scanning.has(company.id)}
                        title={scanning.has(company.id) ? 'Scanning…' : 'Scan now'}
                        className={cn(
                          'p-1.5 rounded-md transition-colors',
                          scanning.has(company.id)
                            ? 'text-violet-400 bg-violet-500/10'
                            : 'text-slate-600 hover:text-violet-300 hover:bg-violet-500/10 opacity-0 group-hover:opacity-100'
                        )}
                      >
                        <RefreshCw size={14} className={scanning.has(company.id) ? 'animate-spin' : ''} />
                      </button>
                      <button
                        onClick={() => setEditCompany(company)}
                        title="Edit"
                        className="p-1.5 text-slate-600 hover:text-white hover:bg-white/[0.06] rounded-md transition-colors opacity-0 group-hover:opacity-100"
                      >
                        <Pencil size={14} />
                      </button>
                      <button
                        onClick={() => handleDelete(company.id)}
                        title="Delete"
                        className="p-1.5 text-slate-600 hover:text-red-400 hover:bg-red-500/10 rounded-md transition-colors opacity-0 group-hover:opacity-100"
                      >
                        <Trash2 size={14} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <AddCompanyModal
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onCreated={c => setCompanies(cs => [...cs, c])}
      />

      <AddCompanyModal
        open={!!editCompany}
        initialData={editCompany ?? undefined}
        onClose={() => setEditCompany(null)}
        onUpdated={updated => {
          setCompanies(cs => cs.map(c => c.id === updated.id ? updated : c))
          toast('Company updated')
        }}
      />

      <ScanStatsDrawer
        company={drawerCompany}
        onClose={() => setDrawerCompany(null)}
      />

      {/* Toast */}
      {toastMsg && (
        <div className="fixed bottom-6 left-1/2 -translate-x-1/2 bg-[#1e1e2a] border border-white/10 text-sm text-white px-4 py-2.5 rounded-lg shadow-xl z-50 animate-fade-in">
          {toastMsg}
        </div>
      )}
    </div>
  )
}

function ScanStatusBadge({ status }: { status: Company['last_scan_status'] }) {
  if (!status) return <span className="text-xs text-slate-500 flex items-center gap-1"><Minus size={10} /> Never</span>
  const map = {
    ok: { icon: <CheckCircle2 size={12} />, label: 'OK', cls: 'text-emerald-400' },
    error: { icon: <XCircle size={12} />, label: 'Error', cls: 'text-red-400' },
    pending: { icon: <Clock size={12} />, label: 'Pending', cls: 'text-amber-400' },
  }
  const { icon, label, cls } = map[status]
  return (
    <span className={cn('flex items-center gap-1 text-xs font-medium', cls)}>
      {icon}{label}
    </span>
  )
}
