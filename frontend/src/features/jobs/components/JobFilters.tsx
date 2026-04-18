import { Search } from 'lucide-react'
import { cn } from '@/lib/utils'
import { MultiSelectDropdown } from '@/components/ui/multi-select-dropdown'
import type { DateFilter } from '@/features/jobs/hooks/useJobs'

// ── FilterPill ─────────────────────────────────────────────────────────────────

function FilterPill({
  active,
  color = 'violet',
  onClick,
  children,
}: {
  active: boolean
  color?: 'violet' | 'emerald'
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'px-2.5 py-1.5 text-xs rounded-lg border transition-colors',
        active && color === 'violet' && 'bg-violet-500/15 border-violet-500/30 text-violet-300',
        active && color === 'emerald' && 'bg-emerald-500/15 border-emerald-500/30 text-emerald-300',
        !active &&
          'bg-white/[0.03] border-white/[0.07] text-slate-400 hover:text-slate-200 hover:bg-white/[0.06]'
      )}
    >
      {children}
    </button>
  )
}

function Divider() {
  return <div className="w-px h-4 bg-white/[0.08] mx-0.5" />
}

// ── JobFilters ─────────────────────────────────────────────────────────────────

interface JobFiltersProps {
  searchQuery: string
  onSearchChange: (q: string) => void
  companies: string[]
  selectedCompanies: string[]
  onCompaniesChange: (v: string[]) => void
  remoteFilter: boolean
  onRemoteChange: (v: boolean) => void
  onSiteFilter: boolean
  onOnSiteChange: (v: boolean) => void
  expFilter: string[]
  onExpChange: (v: string[]) => void
  dateFilter: DateFilter
  onDateChange: (v: DateFilter) => void
  onReset: () => void
}

const EXP_OPTIONS = ['0-2', '2-5', '5+'] as const
const DATE_OPTIONS: { value: DateFilter; label: string }[] = [
  { value: 'all', label: 'Any' },
  { value: '7d', label: '7d' },
  { value: '30d', label: '30d' },
]

export function JobFilters({
  searchQuery,
  onSearchChange,
  companies,
  selectedCompanies,
  onCompaniesChange,
  remoteFilter,
  onRemoteChange,
  onSiteFilter,
  onOnSiteChange,
  expFilter,
  onExpChange,
  dateFilter,
  onDateChange,
  onReset,
}: JobFiltersProps) {
  function toggleExp(opt: string) {
    onExpChange(expFilter.includes(opt) ? expFilter.filter((v) => v !== opt) : [...expFilter, opt])
  }

  return (
    <div className="flex items-center gap-2 px-6 py-2.5 border-b border-white/[0.07] flex-wrap">
      <div className="relative">
        <Search
          className="absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-500 pointer-events-none"
          size={13}
        />
        <input
          value={searchQuery}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder="Search…"
          className="pl-7 pr-3 py-1.5 text-xs bg-white/5 border border-white/10 rounded-lg placeholder-slate-500 focus:outline-none focus:border-violet-500 w-44 text-slate-200"
        />
      </div>

      <MultiSelectDropdown
        aria-label="Filter by company"
        options={companies}
        selected={selectedCompanies}
        onChange={onCompaniesChange}
        placeholder="All companies"
        singularLabel={(n) => `${n} companies`}
        searchPlaceholder="Search companies…"
      />

      <Divider />

      {EXP_OPTIONS.map((opt) => (
        <FilterPill key={opt} active={expFilter.includes(opt)} onClick={() => toggleExp(opt)}>
          {opt === '5+' ? '5+ yrs' : `${opt} yrs`}
        </FilterPill>
      ))}

      <Divider />

      {DATE_OPTIONS.map((d) => (
        <FilterPill
          key={d.value}
          active={dateFilter === d.value}
          onClick={() => onDateChange(d.value)}
        >
          {d.label}
        </FilterPill>
      ))}

      <Divider />

      <FilterPill
        color="emerald"
        active={remoteFilter}
        onClick={() => onRemoteChange(!remoteFilter)}
      >
        Remote
      </FilterPill>

      <FilterPill
        color="violet"
        active={onSiteFilter}
        onClick={() => onOnSiteChange(!onSiteFilter)}
      >
        On-site
      </FilterPill>

      <Divider />

      <button onClick={onReset} className="text-[11px] text-slate-500 hover:text-slate-300 px-2">
        Reset
      </button>
    </div>
  )
}
