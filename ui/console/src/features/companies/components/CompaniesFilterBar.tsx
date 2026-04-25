import { Search, Filter, Power, Loader2, X, Plus, Play } from 'lucide-react'
import { SegmentedSwitch } from '@/components/ui/segmented-switch'
import { MultiSelectDropdown } from '@/components/ui/multi-select-dropdown'
import type { ActiveFilter, StatusFilter } from '@/features/companies/hooks/useCompanies'
import { STATUS_FILTER, ACTIVE_FILTER } from '@/features/companies/constants'

interface CompaniesFilterBarProps {
  search: string
  onSearchChange: (v: string) => void

  adapters: string[]
  onAdaptersChange: (v: string[]) => void
  adapterOptions: string[]
  onAddAdapter: (v: string) => void

  activeFilter: ActiveFilter
  onActiveFilterChange: (v: ActiveFilter) => void

  statusFilter: StatusFilter
  onStatusFilterChange: (v: StatusFilter) => void

  bulkToggling: boolean
  bulkAllActive: boolean
  onBulkToggle: () => void
  runningAll: boolean
  onRunAll: () => void
  onReset: () => void

  onAddClick: () => void
}

function Divider() {
  return <div className="w-px h-[22px] bg-white/[0.08] mx-1 shrink-0" />
}

export function CompaniesFilterBar(p: CompaniesFilterBarProps) {
  return (
    <div className="px-6 py-3 border-b border-white/[0.07] flex items-center gap-2">
      {/* Left: filters */}
      <div className="flex items-center gap-2 flex-wrap">
        <div className="relative">
          <Search size={13} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-500" />
          <input
            value={p.search}
            onChange={(e) => p.onSearchChange(e.target.value)}
            placeholder="Search companies…"
            className="pl-7 pr-7 py-1.5 text-xs bg-white/5 border border-white/10 rounded-lg placeholder-slate-500 focus:outline-none focus:border-violet-500 w-56 text-white"
          />
          {p.search && (
            <button
              type="button"
              onClick={() => p.onSearchChange('')}
              className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300"
            >
              <X size={12} />
            </button>
          )}
        </div>

        <MultiSelectDropdown
          aria-label="Filter by adapter"
          options={p.adapterOptions}
          selected={p.adapters}
          onChange={p.onAdaptersChange}
          placeholder="All adapters"
          singularLabel={(n) => `${n} adapters`}
          searchPlaceholder="Search adapters…"
          addable
          onAdd={p.onAddAdapter}
          addPlaceholder="Add new adapter…"
          leadingIcon={<Filter size={12} />}
        />

        <Divider />

        <SegmentedSwitch
          aria-label="Active state"
          value={p.activeFilter}
          onChange={p.onActiveFilterChange}
          options={[
            { value: ACTIVE_FILTER.ALL, label: 'All', accent: 'violet' },
            { value: ACTIVE_FILTER.ACTIVE, label: 'Active', accent: 'emerald' },
            { value: ACTIVE_FILTER.INACTIVE, label: 'Inactive', accent: 'violet' },
          ]}
        />

        <Divider />

        <SegmentedSwitch
          aria-label="Status filter"
          value={p.statusFilter}
          onChange={(v) => p.onStatusFilterChange(p.statusFilter === v ? 'none' : v)}
          options={[
            { value: STATUS_FILTER.ERROR, label: '✕ Errors', accent: 'red' },
            { value: STATUS_FILTER.RUNNING, label: '⟳ Running', accent: 'blue' },
            { value: STATUS_FILTER.QUEUED, label: '· Queued', accent: 'amber' },
          ]}
        />

        <button
          type="button"
          onClick={p.onReset}
          className="text-[11px] text-slate-500 hover:text-slate-300 px-1"
        >
          Reset
        </button>
      </div>

      {/* Center: bulk actions */}
      <div className="flex-1 flex items-center justify-center gap-2">
        <button
          type="button"
          onClick={p.onBulkToggle}
          disabled={p.bulkToggling}
          className="flex items-center gap-1.5 text-xs text-slate-300 bg-white/5 border border-white/10 hover:bg-white/10 px-2.5 py-1.5 rounded-lg transition-colors disabled:opacity-50"
        >
          {p.bulkToggling ? <Loader2 size={12} className="animate-spin" /> : <Power size={12} />}
          {p.bulkAllActive ? 'Deactivate all' : 'Activate all'}
        </button>
        <button
          type="button"
          onClick={p.onRunAll}
          disabled={p.runningAll}
          className="flex items-center gap-1.5 text-xs text-emerald-300 bg-emerald-500/10 border border-emerald-500/20 hover:bg-emerald-500/20 px-2.5 py-1.5 rounded-lg transition-colors disabled:opacity-50"
        >
          {p.runningAll ? <Loader2 size={12} className="animate-spin" /> : <Play size={12} />}
          Run all
        </button>
      </div>

      {/* Right: add company */}
      <button
        type="button"
        onClick={p.onAddClick}
        className="flex items-center gap-1.5 bg-violet-600 hover:bg-violet-500 text-white text-xs font-medium px-3 py-1.5 rounded-lg transition-colors shrink-0"
      >
        <Plus size={12} />
        Add Company
      </button>
    </div>
  )
}
