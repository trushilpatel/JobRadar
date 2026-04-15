import { useState, useEffect, useRef } from 'react'
import { ChevronDown, Check } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { DateFilter } from '@/features/jobs/hooks/useJobs'

// ── CompanyMultiSelect ────────────────────────────────────────────────────────

interface CompanyMultiSelectProps {
  companies: string[]
  selected: string[]
  onChange: (v: string[]) => void
}

function CompanyMultiSelect({ companies, selected, onChange }: CompanyMultiSelectProps) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function onMouseDown(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onMouseDown)
    return () => document.removeEventListener('mousedown', onMouseDown)
  }, [])

  const visible = companies.filter(c =>
    c.toLowerCase().includes(search.toLowerCase())
  )

  function toggle(c: string) {
    onChange(selected.includes(c) ? selected.filter(v => v !== c) : [...selected, c])
  }

  const label =
    selected.length === 0
      ? 'All companies'
      : selected.length === 1
        ? selected[0]
        : `${selected.length} companies`

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(v => !v)}
        className={cn(
          'flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg border transition-colors min-w-[120px]',
          selected.length > 0
            ? 'bg-violet-500/15 border-violet-500/30 text-violet-300'
            : 'bg-white/5 border-white/10 text-slate-300 hover:bg-white/[0.08]',
        )}
      >
        <span className="flex-1 text-left truncate max-w-[140px]">{label}</span>
        <ChevronDown size={11} className="shrink-0 text-slate-500" />
      </button>

      {open && (
        <div className="absolute top-full left-0 mt-1 w-56 bg-[#13131e] border border-white/[0.1] rounded-lg shadow-xl z-20 overflow-hidden">
          <div className="p-2 border-b border-white/[0.07]">
            <input
              autoFocus
              value={search}
              onChange={e => setSearch(e.target.value)}
              placeholder="Search…"
              className="w-full bg-white/5 border border-white/10 rounded-md px-2.5 py-1.5 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-violet-500"
            />
          </div>
          <div className="max-h-52 overflow-y-auto py-1">
            {visible.length === 0 ? (
              <p className="text-xs text-slate-500 px-3 py-2">No results</p>
            ) : (
              visible.map(c => (
                <button
                  key={c}
                  onClick={() => toggle(c)}
                  className={cn(
                    'w-full text-left px-3 py-2 text-xs transition-colors flex items-center gap-2',
                    selected.includes(c)
                      ? 'text-violet-300 bg-violet-500/10'
                      : 'text-slate-300 hover:bg-white/[0.05]',
                  )}
                >
                  <div className={cn(
                    'w-3.5 h-3.5 rounded border flex items-center justify-center shrink-0',
                    selected.includes(c) ? 'bg-violet-500 border-violet-500' : 'border-white/20',
                  )}>
                    {selected.includes(c) && <Check size={9} className="text-white" />}
                  </div>
                  {c}
                </button>
              ))
            )}
          </div>
          {selected.length > 0 && (
            <div className="border-t border-white/[0.07] p-2">
              <button
                onClick={() => { onChange([]); setSearch('') }}
                className="w-full text-xs text-slate-500 hover:text-slate-300 transition-colors py-1"
              >
                Clear selection
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

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
      <CompanyMultiSelect
        companies={companies}
        selected={selectedCompanies}
        onChange={onCompaniesChange}
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
