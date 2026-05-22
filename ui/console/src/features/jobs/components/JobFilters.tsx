import { forwardRef } from 'react'
import { Search, SlidersHorizontal } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useIsMobile } from '@/lib/use-media-query'
import { MultiSelectDropdown } from '@/components/ui/multi-select-dropdown'
import { MobileFilterDrawer } from '@/features/jobs/components/MobileFilterDrawer'
import type { DateFilter, LocationFilter } from '@/features/jobs/hooks/useJobs'

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
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        'px-2.5 py-1.5 text-xs rounded-lg border transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500',
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

interface JobFiltersProps {
  searchQuery: string
  onSearchChange: (q: string) => void
  companies: string[]
  selectedCompanies: string[]
  onCompaniesChange: (v: string[]) => void
  locationFilter: LocationFilter
  onLocationChange: (v: LocationFilter) => void
  expFilter: string[]
  onExpChange: (v: string[]) => void
  dateFilter: DateFilter
  onDateChange: (v: DateFilter) => void
  onReset: () => void
  hasActiveFilters: boolean
  mobileOpen?: boolean
  onMobileOpenChange?: (open: boolean) => void
  activeFilterCount?: number
}

const EXP_OPTIONS = ['0-2', '2-5', '5+'] as const

const DATE_OPTIONS: { value: Exclude<DateFilter, 'all'>; label: string }[] = [
  { value: '24h', label: '24h' },
  { value: '7d', label: '7d' },
  { value: '30d', label: '30d' },
]

const LOCATION_OPTIONS: { value: LocationFilter; label: string; color: 'violet' | 'emerald' }[] = [
  { value: 'remote', label: 'Remote', color: 'emerald' },
  { value: 'onsite', label: 'On-site', color: 'violet' },
]

export const JobFilters = forwardRef<HTMLInputElement, JobFiltersProps>(function JobFilters(
  {
    searchQuery,
    onSearchChange,
    companies,
    selectedCompanies,
    onCompaniesChange,
    locationFilter,
    onLocationChange,
    expFilter,
    onExpChange,
    dateFilter,
    onDateChange,
    onReset,
    hasActiveFilters,
    mobileOpen = false,
    onMobileOpenChange,
    activeFilterCount = 0,
  },
  searchRef
) {
  const isMobile = useIsMobile()

  function toggleExp(opt: string) {
    onExpChange(expFilter.includes(opt) ? expFilter.filter((v) => v !== opt) : [...expFilter, opt])
  }

  function toggleDate(value: Exclude<DateFilter, 'all'>) {
    onDateChange(dateFilter === value ? 'all' : value)
  }

  function toggleLocation(value: LocationFilter) {
    onLocationChange(locationFilter === value ? 'any' : value)
  }

  if (isMobile) {
    return (
      <>
        <div className="flex items-center gap-2 px-4 py-2 border-b border-white/[0.07]">
          <div className="relative flex-1">
            <Search
              className="absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-500 pointer-events-none"
              size={13}
            />
            <input
              ref={searchRef}
              type="search"
              value={searchQuery}
              onChange={(e) => onSearchChange(e.target.value)}
              placeholder="Search…"
              className="w-full min-h-[44px] pl-7 pr-3 py-1.5 text-sm bg-white/5 border border-white/10 rounded-lg placeholder-slate-500 focus:outline-none focus:border-violet-500 text-slate-200"
            />
          </div>
          <button
            type="button"
            onClick={() => onMobileOpenChange?.(true)}
            aria-label="Open filters"
            className={cn(
              'flex min-h-[44px] min-w-[44px] items-center justify-center gap-1.5 rounded-lg border px-3 transition-colors',
              hasActiveFilters
                ? 'border-violet-500/30 bg-violet-500/15 text-violet-300'
                : 'border-white/10 bg-white/5 text-slate-300 hover:bg-white/[0.08]'
            )}
          >
            <SlidersHorizontal size={16} />
            {activeFilterCount > 0 && <span className="text-xs">{activeFilterCount}</span>}
          </button>
        </div>
        <MobileFilterDrawer
          open={mobileOpen}
          onOpenChange={onMobileOpenChange ?? (() => {})}
          companies={companies}
          selectedCompanies={selectedCompanies}
          onCompaniesChange={onCompaniesChange}
          locationFilter={locationFilter}
          onLocationChange={onLocationChange}
          expFilter={expFilter}
          onExpChange={onExpChange}
          dateFilter={dateFilter}
          onDateChange={onDateChange}
          onReset={onReset}
        />
      </>
    )
  }

  return (
    <div className="flex items-center gap-2 px-6 py-2.5 border-b border-white/[0.07] flex-wrap">
      <div className="relative">
        <Search
          className="absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-500 pointer-events-none"
          size={13}
        />
        <input
          ref={searchRef}
          type="search"
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
          onClick={() => toggleDate(d.value)}
        >
          {d.label}
        </FilterPill>
      ))}

      <Divider />

      {LOCATION_OPTIONS.map((loc) => (
        <FilterPill
          key={loc.value}
          color={loc.color}
          active={locationFilter === loc.value}
          onClick={() => toggleLocation(loc.value)}
        >
          {loc.label}
        </FilterPill>
      ))}

      {hasActiveFilters && (
        <>
          <Divider />
          <button
            type="button"
            aria-label="Reset filters"
            onClick={onReset}
            className="text-[11px] text-slate-500 hover:text-slate-300 px-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500 rounded"
          >
            Reset
          </button>
        </>
      )}
    </div>
  )
})
