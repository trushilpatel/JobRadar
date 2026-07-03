import { forwardRef } from 'react'
import { Search, SlidersHorizontal } from 'lucide-react'
import { cn } from '@/lib/utils/utils'
import { useIsMobile } from '@/lib/hooks/use-media-query'
import { MobileFilterDrawer } from './MobileFilterDrawer'
import { FilterPopover } from './FilterPopover'
import { FilterChip } from './FilterChip'
import type { DateFilter, LocationFilter } from '../hooks/useJobs'
import type { VisaFilter } from '../hooks/useJobUrlState'
import { COPY } from '@/config/copy'
import {
  EXP_OPTIONS,
  DATE_OPTIONS,
  LOCATION_OPTIONS,
  VISA_OPTIONS,
  pickSingleSelectValue,
} from '../constants/filterOptions'

function Divider() {
  return <div className="w-px h-4 bg-[var(--border-strong)] mx-0.5" />
}

interface FilterChipDescriptor {
  key: string
  label: string
  onRemove: () => void
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
  visaFilter: VisaFilter
  onVisaChange: (v: VisaFilter) => void
  onReset: () => void
  hasActiveFilters: boolean
  mobileOpen?: boolean
  onMobileOpenChange?: (open: boolean) => void
  activeFilterCount?: number
}

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
    visaFilter,
    onVisaChange,
    onReset,
    hasActiveFilters,
    mobileOpen = false,
    onMobileOpenChange,
    activeFilterCount = 0,
  },
  searchRef
) {
  const isMobile = useIsMobile()

  if (isMobile) {
    return (
      <>
        <div className="flex items-center gap-2 px-4 py-2 border-b border-[var(--border)]">
          <div className="relative flex-1">
            <Search
              className="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted pointer-events-none"
              size={13}
            />
            <input
              ref={searchRef}
              type="search"
              value={searchQuery}
              onChange={(e) => onSearchChange(e.target.value)}
              placeholder={COPY['jobs.search.placeholder']}
              className="w-full min-h-[44px] pl-7 pr-3 py-1.5 text-sm bg-subtle border border-[var(--border)] rounded-lg placeholder:text-placeholder focus:outline-none focus:border-violet-500 text-primary"
            />
          </div>
          <button
            type="button"
            onClick={() => onMobileOpenChange?.(true)}
            aria-label="Open filters"
            className={cn(
              'flex min-h-[44px] min-w-[44px] items-center justify-center gap-1.5 rounded-lg border px-3 transition-colors',
              hasActiveFilters
                ? 'border-violet-400/50 dark:border-violet-500/30 bg-violet-100 dark:bg-violet-500/15 text-violet-700 dark:text-violet-300'
                : 'border-[var(--border-strong)] bg-subtle text-secondary hover:bg-hover'
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
          visaFilter={visaFilter}
          onVisaChange={onVisaChange}
          onReset={onReset}
        />
      </>
    )
  }

  // ── Active-filter chips ─────────────────────────────────────────────────────
  // One chip per active facet value (company, location, experience, date,
  // visa — search is excluded, chips are for discrete facets only). Each
  // chip's onRemove clears JUST that value, never the whole filter set.
  const chips: FilterChipDescriptor[] = []

  for (const company of selectedCompanies) {
    chips.push({
      key: `company-${company}`,
      label: company,
      onRemove: () => onCompaniesChange(selectedCompanies.filter((c) => c !== company)),
    })
  }

  if (locationFilter !== 'any') {
    const opt = LOCATION_OPTIONS.find((o) => o.value === locationFilter)
    chips.push({
      key: 'location',
      label: opt?.label ?? locationFilter,
      onRemove: () => onLocationChange('any'),
    })
  }

  for (const exp of expFilter) {
    const opt = EXP_OPTIONS.find((o) => o.value === exp)
    chips.push({
      key: `exp-${exp}`,
      label: opt?.label ?? exp,
      onRemove: () => onExpChange(expFilter.filter((v) => v !== exp)),
    })
  }

  if (dateFilter !== 'all') {
    const opt = DATE_OPTIONS.find((o) => o.value === dateFilter)
    chips.push({
      key: 'date',
      label: opt?.label ?? dateFilter,
      onRemove: () => onDateChange('all'),
    })
  }

  if (visaFilter !== 'any') {
    const opt = VISA_OPTIONS.find((o) => o.value === visaFilter)
    chips.push({
      key: 'visa',
      label: opt?.label ?? visaFilter,
      onRemove: () => onVisaChange('any'),
    })
  }

  return (
    <div className="flex flex-col border-b border-[var(--border)]">
      <div className="flex items-center gap-2 px-6 py-2.5 flex-wrap">
        <div className="relative">
          <Search
            className="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted pointer-events-none"
            size={13}
          />
          <input
            ref={searchRef}
            type="search"
            value={searchQuery}
            onChange={(e) => onSearchChange(e.target.value)}
            placeholder={COPY['jobs.search.placeholder']}
            className="pl-7 pr-3 py-1.5 text-xs bg-subtle border border-[var(--border)] rounded-lg placeholder:text-placeholder focus:outline-none focus:border-violet-500 w-44 text-primary"
          />
        </div>

        <FilterPopover
          label="Company"
          options={companies.map((c) => ({ value: c, label: c }))}
          selected={selectedCompanies}
          onChange={onCompaniesChange}
          searchable
          showCount
          emptyLabel="No companies"
        />

        <Divider />

        <FilterPopover
          label="Location"
          options={LOCATION_OPTIONS}
          selected={locationFilter === 'any' ? [] : [locationFilter]}
          onChange={(next) => onLocationChange(pickSingleSelectValue(next, locationFilter, 'any'))}
        />

        <FilterPopover
          label="Experience"
          options={EXP_OPTIONS}
          selected={expFilter}
          onChange={onExpChange}
        />

        <FilterPopover
          label="Posted"
          options={DATE_OPTIONS}
          selected={dateFilter === 'all' ? [] : [dateFilter]}
          onChange={(next) =>
            onDateChange(pickSingleSelectValue(next, dateFilter, 'all') as DateFilter)
          }
        />

        <FilterPopover
          label="Visa"
          options={VISA_OPTIONS}
          selected={visaFilter === 'any' ? [] : [visaFilter]}
          onChange={(next) =>
            onVisaChange(pickSingleSelectValue(next, visaFilter, 'any') as VisaFilter)
          }
        />

        {hasActiveFilters && (
          <>
            <Divider />
            <button
              type="button"
              aria-label="Reset filters"
              onClick={onReset}
              className="text-[11px] text-muted hover:text-primary px-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500 rounded"
            >
              Reset
            </button>
          </>
        )}
      </div>

      {chips.length > 0 && (
        <div className="flex items-center gap-1.5 px-6 pb-2.5 flex-wrap">
          {chips.map((chip) => (
            <FilterChip key={chip.key} label={chip.label} onRemove={chip.onRemove} />
          ))}
          {chips.length > 1 && (
            <button
              type="button"
              onClick={onReset}
              className="text-[11px] text-muted hover:text-primary px-1.5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500 rounded"
            >
              Clear all
            </button>
          )}
        </div>
      )}
    </div>
  )
})
