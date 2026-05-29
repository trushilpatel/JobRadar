import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from '@/components/ui/drawer'
import { MultiSelectDropdown } from '@/components/ui/multi-select-dropdown'
import { cn } from '@/lib/utils/utils'
import type { DateFilter, LocationFilter } from '@/features/jobs/hooks/useJobs'

export interface MobileFilterDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  companies?: string[]
  selectedCompanies?: string[]
  onCompaniesChange?: (v: string[]) => void
  locationFilter?: LocationFilter
  onLocationChange?: (v: LocationFilter) => void
  expFilter?: string[]
  onExpChange?: (v: string[]) => void
  dateFilter?: DateFilter
  onDateChange?: (v: DateFilter) => void
  onReset?: () => void
}

const EXP_OPTIONS = ['0-2', '2-5', '5+'] as const
const DATE_OPTIONS: { value: Exclude<DateFilter, 'all'>; label: string }[] = [
  { value: '24h', label: '24h' },
  { value: '7d', label: '7d' },
  { value: '30d', label: '30d' },
]
const LOCATION_OPTIONS: { value: LocationFilter; label: string }[] = [
  { value: 'remote', label: 'Remote' },
  { value: 'onsite', label: 'On-site' },
]

function PillButton({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'min-h-[44px] px-4 py-2 text-sm rounded-lg border transition-colors',
        active
          ? 'bg-violet-500/15 border-violet-500/30 text-violet-300'
          : 'bg-white/[0.03] border-white/[0.07] text-slate-300 hover:bg-white/[0.06]'
      )}
    >
      {children}
    </button>
  )
}

export function MobileFilterDrawer({
  open,
  onOpenChange,
  companies = [],
  selectedCompanies = [],
  onCompaniesChange,
  locationFilter = 'any',
  onLocationChange,
  expFilter = [],
  onExpChange,
  dateFilter = 'all',
  onDateChange,
  onReset,
}: MobileFilterDrawerProps) {
  function toggleExp(opt: string) {
    if (!onExpChange) return
    onExpChange(expFilter.includes(opt) ? expFilter.filter((v) => v !== opt) : [...expFilter, opt])
  }

  function toggleDate(value: Exclude<DateFilter, 'all'>) {
    if (!onDateChange) return
    onDateChange(dateFilter === value ? 'all' : value)
  }

  function toggleLocation(value: LocationFilter) {
    if (!onLocationChange) return
    onLocationChange(locationFilter === value ? 'any' : value)
  }

  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerContent className="border-t border-white/[0.08] bg-[#0d0d14] text-slate-200">
        <DrawerHeader>
          <DrawerTitle className="text-base font-semibold text-white">Filters</DrawerTitle>
        </DrawerHeader>

        <div className="space-y-6 px-4 pb-4">
          {onCompaniesChange && (
            <section>
              <h4 className="mb-2 text-xs font-medium uppercase tracking-wide text-slate-400">
                Companies
              </h4>
              <MultiSelectDropdown
                aria-label="Filter by company"
                options={companies}
                selected={selectedCompanies}
                onChange={onCompaniesChange}
                placeholder="All companies"
                singularLabel={(n) => `${n} companies`}
                searchPlaceholder="Search companies…"
              />
            </section>
          )}

          <section>
            <h4 className="mb-2 text-xs font-medium uppercase tracking-wide text-slate-400">
              Experience
            </h4>
            <div className="flex flex-wrap gap-2">
              {EXP_OPTIONS.map((opt) => (
                <PillButton
                  key={opt}
                  active={expFilter.includes(opt)}
                  onClick={() => toggleExp(opt)}
                >
                  {opt === '5+' ? '5+ yrs' : `${opt} yrs`}
                </PillButton>
              ))}
            </div>
          </section>

          <section>
            <h4 className="mb-2 text-xs font-medium uppercase tracking-wide text-slate-400">
              Posted
            </h4>
            <div className="flex flex-wrap gap-2">
              {DATE_OPTIONS.map((d) => (
                <PillButton
                  key={d.value}
                  active={dateFilter === d.value}
                  onClick={() => toggleDate(d.value)}
                >
                  {d.label}
                </PillButton>
              ))}
            </div>
          </section>

          <section>
            <h4 className="mb-2 text-xs font-medium uppercase tracking-wide text-slate-400">
              Location
            </h4>
            <div className="flex flex-wrap gap-2">
              {LOCATION_OPTIONS.map((loc) => (
                <PillButton
                  key={loc.value}
                  active={locationFilter === loc.value}
                  onClick={() => toggleLocation(loc.value)}
                >
                  {loc.label}
                </PillButton>
              ))}
            </div>
          </section>
        </div>

        <DrawerFooter className="flex-row gap-2 sm:space-x-0">
          {onReset && (
            <button
              type="button"
              onClick={() => {
                onReset()
              }}
              className="flex-1 min-h-[44px] rounded-lg border border-white/10 bg-white/5 px-4 py-2 text-sm text-slate-400 transition-colors hover:bg-white/[0.08]"
            >
              Reset
            </button>
          )}
          <DrawerClose asChild>
            <button
              type="button"
              className="flex-1 min-h-[44px] rounded-lg bg-violet-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-violet-500"
            >
              Done
            </button>
          </DrawerClose>
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  )
}
