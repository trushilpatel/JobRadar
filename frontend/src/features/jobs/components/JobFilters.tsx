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
        !active && 'bg-white/[0.03] border-white/[0.07] text-slate-400 hover:text-slate-200 hover:bg-white/[0.06]',
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
  companies: string[]
  selectedCompanies: string[]
  onCompaniesChange: (v: string[]) => void
  remoteFilter: boolean
  onRemoteChange: (v: boolean) => void
  expFilter: string[]
  onExpChange: (v: string[]) => void
  dateFilter: DateFilter
  onDateChange: (v: DateFilter) => void
  sponsorshipFilter: boolean
  onSponsorshipChange: (v: boolean) => void
  noSponsorshipFilter: boolean
  onNoSponsorshipChange: (v: boolean) => void
  noRequirementsFilter: boolean
  onNoRequirementsChange: (v: boolean) => void
  citizenshipReqFilter: boolean
  onCitizenshipReqChange: (v: boolean) => void
  employTypes: string[]
  onEmployTypesChange: (types: string[]) => void
}

const EXP_OPTIONS = ['0-2', '3-5', '5 <'] as const
const DATE_OPTIONS = ['all', '30d', '7d'] as const
const EMPLOY_TYPE_OPTIONS = [{ value: 'full_time', label: 'Full-time' }]

export function JobFilters({
  companies, selectedCompanies, onCompaniesChange,
  remoteFilter, onRemoteChange,
  expFilter, onExpChange,
  dateFilter, onDateChange,
  sponsorshipFilter, onSponsorshipChange,
  noSponsorshipFilter, onNoSponsorshipChange,
  noRequirementsFilter, onNoRequirementsChange,
  citizenshipReqFilter, onCitizenshipReqChange,
  employTypes, onEmployTypesChange,
}: JobFiltersProps) {
  function toggleExp(opt: string) {
    onExpChange(expFilter.includes(opt) ? expFilter.filter(v => v !== opt) : [...expFilter, opt])
  }

  function toggleEmployType(val: string) {
    onEmployTypesChange(
      employTypes.includes(val) ? employTypes.filter(v => v !== val) : [...employTypes, val]
    )
  }

  return (
    <div className="flex items-center gap-2 px-6 py-2.5 border-b border-white/[0.07] flex-wrap">
      <MultiSelectDropdown
        aria-label="Filter by company"
        options={companies}
        selected={selectedCompanies}
        onChange={onCompaniesChange}
        placeholder="All companies"
        singularLabel={n => `${n} companies`}
        searchPlaceholder="Search companies…"
      />

      <Divider />

      {EXP_OPTIONS.map(opt => (
        <FilterPill key={opt} active={expFilter.includes(opt)} onClick={() => toggleExp(opt)}>
          {opt === '5 <' ? '5+ yrs' : `${opt} yrs`}
        </FilterPill>
      ))}

      <Divider />

      {DATE_OPTIONS.map(d => (
        <FilterPill key={d} active={dateFilter === d} onClick={() => onDateChange(d as DateFilter)}>
          {d === 'all' ? 'All' : d}
        </FilterPill>
      ))}

      <Divider />

      {EMPLOY_TYPE_OPTIONS.map(opt => (
        <FilterPill
          key={opt.value}
          active={employTypes.includes(opt.value)}
          onClick={() => toggleEmployType(opt.value)}
        >
          {opt.label}
        </FilterPill>
      ))}

      <Divider />

      <FilterPill color="emerald" active={remoteFilter} onClick={() => onRemoteChange(!remoteFilter)}>
        Remote
      </FilterPill>

      <FilterPill active={sponsorshipFilter} onClick={() => onSponsorshipChange(!sponsorshipFilter)}>
        Sponsorship
      </FilterPill>

      <FilterPill active={noSponsorshipFilter} onClick={() => onNoSponsorshipChange(!noSponsorshipFilter)}>
        No Sponsorship
      </FilterPill>

      <FilterPill active={noRequirementsFilter} onClick={() => onNoRequirementsChange(!noRequirementsFilter)}>
        No Citizenship Req.
      </FilterPill>

      <FilterPill active={citizenshipReqFilter} onClick={() => onCitizenshipReqChange(!citizenshipReqFilter)}>
        Citizenship Req.
      </FilterPill>
    </div>
  )
}
