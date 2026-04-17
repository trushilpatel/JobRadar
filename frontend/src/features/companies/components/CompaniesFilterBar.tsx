import { Search, Filter, Power, Loader2, X } from 'lucide-react'
import { SegmentedSwitch } from '@/components/ui/segmented-switch'
import { MultiSelectDropdown } from '@/components/ui/multi-select-dropdown'
import type { ActiveFilter, StatusFilter } from '@/features/companies/hooks/useCompanies'

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
  onReset: () => void
}

function Divider() {
  return <div className="w-px h-[22px] bg-white/[0.08] mx-1 shrink-0" />
}

export function CompaniesFilterBar(p: CompaniesFilterBarProps) {
  return (
    <div className="px-6 py-3 border-b border-white/[0.07] flex items-center gap-2 flex-wrap">
      {/* Search */}
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

      {/* Adapters */}
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

      {/* Active state */}
      <SegmentedSwitch
        aria-label="Active state"
        value={p.activeFilter}
        onChange={p.onActiveFilterChange}
        options={[
          { value: 'all', label: 'All', accent: 'violet' },
          { value: 'active', label: 'Active', accent: 'emerald' },
          { value: 'inactive', label: 'Inactive', accent: 'violet' },
        ]}
      />

      <Divider />

      {/* Status */}
      <SegmentedSwitch
        aria-label="Status filter"
        value={p.statusFilter}
        onChange={p.onStatusFilterChange}
        options={[
          { value: 'none', label: 'Any', accent: 'violet' },
          { value: 'error', label: '✕ Errors', accent: 'red' },
          { value: 'running', label: '⟳ Running', accent: 'blue' },
          { value: 'queued', label: '· Queued', accent: 'amber' },
        ]}
      />

      <span className="ml-auto flex items-center gap-3">
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
          onClick={p.onReset}
          className="text-[11px] text-slate-500 hover:text-slate-300"
        >
          Reset
        </button>
      </span>
    </div>
  )
}
