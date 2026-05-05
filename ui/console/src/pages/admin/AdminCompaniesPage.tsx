import { useState, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Settings, CheckCircle2, XCircle, ChevronLeft, ChevronRight, Search, RotateCcw } from 'lucide-react'
import * as Dialog from '@radix-ui/react-dialog'
import { cn } from '@/lib/utils'
import { ADAPTER_TYPES } from '@/types'
import {
  fetchAdminCompanies,
  updateAdminCompanyScanSettings,
  setAllAdminCompaniesActive,
  type AdminCompany,
  type ScanSettingsInput,
} from '@/api/adminCompanies'

const LIMIT = 50

type HasSettingsFilter = 'all' | 'needs_setup' | 'configured'

function formatDate(d: string | null) {
  if (!d) return '—'
  return new Date(d).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })
}

// ── Scan settings form modal ─────────────────────────────────────────────────

interface SettingsModalProps {
  company: AdminCompany | null
  onClose: () => void
}

function SettingsModal({ company, onClose }: SettingsModalProps) {
  const qc = useQueryClient()
  const existing = company?.scan_settings

  const [adapterType, setAdapterType] = useState(existing?.adapter_type ?? '')
  const [adapterConfigRaw, setAdapterConfigRaw] = useState(
    existing?.adapter_config ? JSON.stringify(existing.adapter_config, null, 2) : ''
  )
  const [intervalOverride, setIntervalOverride] = useState<string>(
    existing?.scan_interval_override != null ? String(existing.scan_interval_override) : ''
  )
  const [active, setActive] = useState(existing?.active ?? true)
  const [jsonError, setJsonError] = useState<string | null>(null)

  const save = useMutation({
    mutationFn: (input: ScanSettingsInput) =>
      updateAdminCompanyScanSettings(company!.id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-companies'] })
      onClose()
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    let parsed: Record<string, unknown> | null = null
    if (adapterConfigRaw.trim()) {
      try {
        parsed = JSON.parse(adapterConfigRaw)
      } catch {
        setJsonError('Invalid JSON')
        return
      }
    }
    setJsonError(null)
    const input: ScanSettingsInput = {
      adapter_type: adapterType || undefined,
      adapter_config: parsed,
      scan_interval_override: intervalOverride ? Number(intervalOverride) : null,
      active,
    }
    save.mutate(input)
  }

  return (
    <Dialog.Root open={!!company} onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 z-40" />
        <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-lg bg-[#0f1117] border border-white/[0.08] rounded-xl shadow-2xl p-6 focus:outline-none">
          <Dialog.Title className="text-sm font-semibold text-white mb-1">
            Scan settings — {company?.name}
          </Dialog.Title>
          <Dialog.Description className="text-xs text-slate-500 mb-5">
            Configure how the crawler fetches jobs for this company.
          </Dialog.Description>

          <form onSubmit={handleSubmit} className="space-y-4">
            {/* Adapter type */}
            <div>
              <label className="block text-xs text-slate-400 mb-1.5">Adapter type</label>
              <select
                value={adapterType}
                onChange={(e) => setAdapterType(e.target.value)}
                required
                className="w-full bg-white/[0.04] border border-white/[0.08] text-xs text-slate-200 rounded-lg px-3 py-2 focus:outline-none focus:border-violet-500 transition-colors"
              >
                <option value="">— select —</option>
                {ADAPTER_TYPES.map((t) => (
                  <option key={t} value={t}>{t}</option>
                ))}
              </select>
            </div>

            {/* Adapter config JSON */}
            <div>
              <label className="block text-xs text-slate-400 mb-1.5">
                Adapter config <span className="text-slate-600">(JSON, optional)</span>
              </label>
              <textarea
                value={adapterConfigRaw}
                onChange={(e) => { setAdapterConfigRaw(e.target.value); setJsonError(null) }}
                rows={4}
                placeholder='{"careers_url": "https://…"}'
                className={cn(
                  'w-full bg-white/[0.04] border text-xs text-slate-200 rounded-lg px-3 py-2 font-mono resize-y focus:outline-none transition-colors',
                  jsonError ? 'border-red-500/60' : 'border-white/[0.08] focus:border-violet-500'
                )}
              />
              {jsonError && <p className="text-xs text-red-400 mt-1">{jsonError}</p>}
            </div>

            {/* Scan interval override */}
            <div>
              <label className="block text-xs text-slate-400 mb-1.5">
                Scan interval override <span className="text-slate-600">(minutes, leave blank for global default)</span>
              </label>
              <input
                type="number"
                min={1}
                value={intervalOverride}
                onChange={(e) => setIntervalOverride(e.target.value)}
                placeholder="1440"
                className="w-full bg-white/[0.04] border border-white/[0.08] text-xs text-slate-200 rounded-lg px-3 py-2 focus:outline-none focus:border-violet-500 transition-colors"
              />
            </div>

            {/* Active toggle */}
            <div className="flex items-center justify-between py-1">
              <span className="text-xs text-slate-400">Active (include in scan schedule)</span>
              <button
                type="button"
                onClick={() => setActive((v) => !v)}
                className={cn(
                  'relative inline-flex h-5 w-9 rounded-full transition-colors',
                  active ? 'bg-violet-600' : 'bg-white/10'
                )}
              >
                <span
                  className={cn(
                    'absolute top-0.5 left-0.5 h-4 w-4 rounded-full bg-white shadow transition-transform',
                    active ? 'translate-x-4' : 'translate-x-0'
                  )}
                />
              </button>
            </div>

            <div className="flex items-center justify-end gap-3 pt-2 border-t border-white/[0.06]">
              <Dialog.Close asChild>
                <button
                  type="button"
                  className="text-xs text-slate-400 hover:text-slate-200 transition-colors px-3 py-1.5"
                >
                  Cancel
                </button>
              </Dialog.Close>
              <button
                type="submit"
                disabled={save.isPending}
                className="text-xs bg-violet-600 hover:bg-violet-500 disabled:opacity-50 text-white rounded-lg px-4 py-1.5 transition-colors"
              >
                {save.isPending ? 'Saving…' : existing ? 'Update' : 'Save'}
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

// ── Main page ────────────────────────────────────────────────────────────────

export function AdminCompaniesPage() {
  const qc = useQueryClient()
  const [search, setSearch] = useState('')
  const [hasSettingsFilter, setHasSettingsFilter] = useState<HasSettingsFilter>('all')
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<AdminCompany | null>(null)

  const queryFilters = useMemo(() => ({
    search: search || undefined,
    has_settings: hasSettingsFilter === 'all'
      ? undefined
      : hasSettingsFilter === 'configured',
  }), [search, hasSettingsFilter])

  const { data, isLoading } = useQuery({
    queryKey: ['admin-companies', queryFilters, page],
    queryFn: () => fetchAdminCompanies(queryFilters, page, LIMIT),
    staleTime: 30_000,
  })

  const setAllActive = useMutation({
    mutationFn: (active: boolean) => setAllAdminCompaniesActive(active),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin-companies'] }),
  })

  const companies = data?.data ?? []
  const total = data?.total ?? 0
  const totalActive = data?.total_active ?? 0
  const hasNext = page * LIMIT < total
  const hasPrev = page > 1

  const needsSetup = companies.filter((c) => !c.scan_settings).length

  return (
    <div className="flex-1 flex flex-col min-h-0 p-6 gap-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold text-white">Companies</h1>
          <p className="text-xs text-slate-500 mt-0.5">
            {totalActive} active · {data ? total - totalActive : '—'} inactive
            {needsSetup > 0 && (
              <span className="text-amber-400 ml-2">· {needsSetup} need setup</span>
            )}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setAllActive.mutate(true)}
            disabled={setAllActive.isPending}
            className="text-xs text-slate-400 hover:text-white bg-white/[0.04] hover:bg-white/[0.08] border border-white/[0.08] px-3 py-1.5 rounded-lg transition-colors disabled:opacity-40"
          >
            Enable all
          </button>
          <button
            onClick={() => setAllActive.mutate(false)}
            disabled={setAllActive.isPending}
            className="text-xs text-slate-400 hover:text-white bg-white/[0.04] hover:bg-white/[0.08] border border-white/[0.08] px-3 py-1.5 rounded-lg transition-colors disabled:opacity-40"
          >
            Disable all
          </button>
        </div>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3">
        <div className="relative flex-1 max-w-xs">
          <Search size={13} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500 pointer-events-none" />
          <input
            value={search}
            onChange={(e) => { setSearch(e.target.value); setPage(1) }}
            placeholder="Search companies…"
            className="w-full bg-white/[0.04] border border-white/[0.08] text-xs text-slate-200 rounded-lg pl-8 pr-3 py-2 focus:outline-none focus:border-violet-500 transition-colors"
          />
        </div>
        <div className="flex rounded-lg overflow-hidden border border-white/[0.08]">
          {(['all', 'needs_setup', 'configured'] as HasSettingsFilter[]).map((v) => (
            <button
              key={v}
              onClick={() => { setHasSettingsFilter(v); setPage(1) }}
              className={cn(
                'text-xs px-3 py-1.5 transition-colors',
                hasSettingsFilter === v
                  ? 'bg-violet-600 text-white'
                  : 'text-slate-400 hover:text-white bg-white/[0.02] hover:bg-white/[0.06]'
              )}
            >
              {v === 'all' ? 'All' : v === 'needs_setup' ? 'Needs setup' : 'Configured'}
            </button>
          ))}
        </div>
        {(search || hasSettingsFilter !== 'all') && (
          <button
            onClick={() => { setSearch(''); setHasSettingsFilter('all'); setPage(1) }}
            className="text-xs text-slate-500 hover:text-slate-200 transition-colors"
          >
            <RotateCcw size={13} />
          </button>
        )}
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto rounded-xl border border-white/[0.07]">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-white/[0.07]">
              <th className="text-left text-slate-500 font-medium px-4 py-3">Company</th>
              <th className="text-left text-slate-500 font-medium px-4 py-3">Adapter</th>
              <th className="text-left text-slate-500 font-medium px-4 py-3">Status</th>
              <th className="text-left text-slate-500 font-medium px-4 py-3">Last scan</th>
              <th className="text-left text-slate-500 font-medium px-4 py-3">Errors</th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody>
            {isLoading && (
              <tr>
                <td colSpan={6} className="px-4 py-8 text-center text-slate-600">Loading…</td>
              </tr>
            )}
            {!isLoading && companies.length === 0 && (
              <tr>
                <td colSpan={6} className="px-4 py-8 text-center text-slate-600">No companies match</td>
              </tr>
            )}
            {companies.map((c) => {
              const s = c.scan_settings
              return (
                <tr
                  key={c.id}
                  className="border-b border-white/[0.04] hover:bg-white/[0.02] transition-colors"
                >
                  <td className="px-4 py-3 text-slate-200 font-medium">{c.name}</td>
                  <td className="px-4 py-3 text-slate-400">{s?.adapter_type ?? '—'}</td>
                  <td className="px-4 py-3">
                    {!s ? (
                      <span className="inline-flex items-center gap-1 text-amber-400">
                        <XCircle size={12} /> Needs setup
                      </span>
                    ) : s.active ? (
                      <span className="inline-flex items-center gap-1 text-emerald-400">
                        <CheckCircle2 size={12} /> Active
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1 text-slate-500">
                        <XCircle size={12} /> Inactive
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-slate-500">{formatDate(s?.last_scan_at ?? null)}</td>
                  <td className="px-4 py-3">
                    {s && s.consecutive_errors > 0 ? (
                      <span className="text-red-400">{s.consecutive_errors}</span>
                    ) : (
                      <span className="text-slate-600">—</span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button
                      onClick={() => setSelected(c)}
                      className="inline-flex items-center gap-1.5 text-xs text-slate-400 hover:text-white bg-white/[0.04] hover:bg-white/[0.08] border border-white/[0.08] px-2.5 py-1 rounded-lg transition-colors"
                    >
                      <Settings size={11} />
                      {s ? 'Edit' : 'Set up'}
                    </button>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      <div className="flex items-center justify-between shrink-0">
        <span className="text-xs text-slate-600">
          {total > 0 ? `${(page - 1) * LIMIT + 1}–${Math.min(page * LIMIT, total)} of ${total}` : '0 results'}
        </span>
        <div className="flex items-center gap-1">
          <button
            onClick={() => setPage((p) => p - 1)}
            disabled={!hasPrev}
            className="flex items-center gap-1 px-2.5 py-1.5 text-xs text-slate-400 hover:text-white hover:bg-white/[0.06] rounded-lg transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
          >
            <ChevronLeft size={13} /> Prev
          </button>
          <span className="px-2 text-xs text-slate-500">Page {page}</span>
          <button
            onClick={() => setPage((p) => p + 1)}
            disabled={!hasNext}
            className="flex items-center gap-1 px-2.5 py-1.5 text-xs text-slate-400 hover:text-white hover:bg-white/[0.06] rounded-lg transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
          >
            Next <ChevronRight size={13} />
          </button>
        </div>
      </div>

      <SettingsModal company={selected} onClose={() => setSelected(null)} />
    </div>
  )
}
