import { useState } from 'react'
import {
  Search,
  Filter,
  Power,
  Loader2,
  X,
  Plus,
  Play,
  XCircle,
  RefreshCw,
  Clock,
  FilterX,
  CheckCircle2,
} from 'lucide-react'
import * as Dialog from '@radix-ui/react-dialog'
import { cn } from '@/lib/utils/utils'
import { SegmentedSwitch } from '@/components/ui/segmented-switch'
import { MultiSelectDropdown } from '@/components/ui/multi-select-dropdown'
import type { ActiveFilter, ScanStatusFilter } from '../constants'
import { SCAN_STATUS_FILTER, ACTIVE_FILTER } from '../constants'

interface CompaniesFilterBarProps {
  search: string
  onSearchChange: (v: string) => void

  adapters: string[]
  onAdaptersChange: (v: string[]) => void
  adapterOptions: string[]
  onAddAdapter: (v: string) => void

  activeFilter: ActiveFilter
  onActiveFilterChange: (v: ActiveFilter) => void

  scanStatus: ScanStatusFilter
  onScanStatusChange: (v: ScanStatusFilter) => void

  bulkToggling: boolean
  bulkAllActive: boolean
  onBulkToggle: () => void
  runningAll: boolean
  onRunAll: () => void

  hasActiveFilters: boolean
  onReset: () => void

  onAddClick: () => void
}

const SCAN_STATUS_OPTIONS: {
  value: ScanStatusFilter
  label: React.ReactNode
  activeClass: string
}[] = [
  {
    value: SCAN_STATUS_FILTER.OK,
    label: (
      <span className="flex items-center gap-1">
        <CheckCircle2 size={11} />
        OK
      </span>
    ),
    activeClass: 'text-emerald-300 bg-emerald-500/20 border-emerald-500/40',
  },
  {
    value: SCAN_STATUS_FILTER.ERROR,
    label: (
      <span className="flex items-center gap-1">
        <XCircle size={11} />
        Errors
      </span>
    ),
    activeClass: 'text-red-300 bg-red-500/20 border-red-500/40',
  },
  {
    value: SCAN_STATUS_FILTER.RUNNING,
    label: (
      <span className="flex items-center gap-1">
        <RefreshCw size={11} />
        Running
      </span>
    ),
    activeClass: 'text-blue-300 bg-blue-500/20 border-blue-500/40',
  },
  {
    value: SCAN_STATUS_FILTER.QUEUED,
    label: (
      <span className="flex items-center gap-1">
        <Clock size={11} />
        Queued
      </span>
    ),
    activeClass: 'text-amber-300 bg-amber-500/20 border-amber-500/40',
  },
]

function Divider() {
  return <div className="w-px h-[22px] bg-[var(--border-strong)] mx-1 shrink-0" />
}

export function CompaniesFilterBar(p: CompaniesFilterBarProps) {
  const [deactivateConfirmOpen, setDeactivateConfirmOpen] = useState(false)
  const [runAllConfirmOpen, setRunAllConfirmOpen] = useState(false)

  return (
    <div className="px-6 py-3 border-b border-[var(--border)] flex items-center gap-2">
      {/* Left: filters */}
      <div className="flex items-center gap-2 flex-wrap">
        {/* Search */}
        <div className="relative">
          <Search size={13} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted" />
          <input
            value={p.search}
            onChange={(e) => p.onSearchChange(e.target.value)}
            placeholder="Search companies…"
            className="pl-7 pr-7 py-1.5 text-xs bg-subtle border border-[var(--border-strong)] rounded-lg placeholder:text-placeholder focus:outline-none focus:border-violet-500 w-52 text-primary"
          />
          {p.search && (
            <button
              type="button"
              onClick={() => p.onSearchChange('')}
              className="absolute right-2 top-1/2 -translate-y-1/2 text-muted hover:text-secondary"
            >
              <X size={12} />
            </button>
          )}
        </div>

        {/* Adapter multi-select */}
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

        {/* Active filter */}
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

        {/* Scan status filter pills — single select, click again to deselect */}
        <div className="flex items-center gap-1">
          {SCAN_STATUS_OPTIONS.map((opt) => {
            const isActive = p.scanStatus === opt.value
            return (
              <button
                key={opt.value}
                type="button"
                onClick={() => p.onScanStatusChange(isActive ? SCAN_STATUS_FILTER.ALL : opt.value)}
                className={cn(
                  'flex items-center gap-1 px-2.5 py-1 text-[11px] font-medium rounded-full border transition-colors',
                  isActive
                    ? opt.activeClass
                    : 'text-muted bg-subtle border-[var(--border-strong)] hover:border-[var(--border-strong)] hover:text-secondary'
                )}
              >
                {opt.label}
              </button>
            )
          })}
        </div>

        {/* Reset */}
        <button
          type="button"
          onClick={p.onReset}
          className={cn(
            'flex items-center gap-1 text-[11px] text-muted hover:text-secondary px-1 transition-opacity',
            p.hasActiveFilters ? 'opacity-100' : 'opacity-0 pointer-events-none'
          )}
          title="Reset filters"
        >
          <FilterX size={12} />
          Reset
        </button>
      </div>

      {/* Center: bulk actions */}
      <div className="flex-1 flex items-center justify-center gap-2">
        <button
          type="button"
          onClick={p.bulkAllActive ? () => setDeactivateConfirmOpen(true) : p.onBulkToggle}
          disabled={p.bulkToggling}
          className="flex items-center gap-1.5 text-xs text-secondary bg-subtle border border-[var(--border-strong)] hover:bg-hover px-2.5 py-1.5 rounded-lg transition-colors disabled:opacity-50"
        >
          {p.bulkToggling ? <Loader2 size={12} className="animate-spin" /> : <Power size={12} />}
          {p.bulkAllActive ? 'Deactivate all' : 'Activate all'}
        </button>
        <button
          type="button"
          onClick={() => setRunAllConfirmOpen(true)}
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

      {/* Deactivate all confirm */}
      <Dialog.Root open={deactivateConfirmOpen} onOpenChange={setDeactivateConfirmOpen}>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 bg-black/60 z-50" />
          <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-sm bg-panel border border-[var(--border-strong)] rounded-xl shadow-2xl p-6">
            <Dialog.Title className="text-base font-semibold text-primary mb-2">
              Deactivate all companies?
            </Dialog.Title>
            <Dialog.Description className="text-sm text-muted mb-5">
              This will stop all future scans for every active company. You can reactivate them
              individually or all at once.
            </Dialog.Description>
            <div className="flex gap-3">
              <button
                type="button"
                onClick={() => setDeactivateConfirmOpen(false)}
                className="flex-1 px-4 py-2 rounded-lg text-sm font-medium bg-subtle hover:bg-hover text-secondary transition-colors"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={() => {
                  setDeactivateConfirmOpen(false)
                  p.onBulkToggle()
                }}
                className="flex-1 px-4 py-2 rounded-lg text-sm font-medium bg-red-600 hover:bg-red-500 text-white transition-colors"
              >
                Deactivate all
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>

      {/* Run all confirm */}
      <Dialog.Root open={runAllConfirmOpen} onOpenChange={setRunAllConfirmOpen}>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 bg-black/60 z-50" />
          <Dialog.Content className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-sm bg-panel border border-[var(--border-strong)] rounded-xl shadow-2xl p-6">
            <Dialog.Title className="text-base font-semibold text-primary mb-2">
              Queue all companies for scan?
            </Dialog.Title>
            <Dialog.Description className="text-sm text-muted mb-5">
              This will queue all active companies for an immediate scan. This may generate
              significant load.
            </Dialog.Description>
            <div className="flex gap-3">
              <button
                type="button"
                onClick={() => setRunAllConfirmOpen(false)}
                className="flex-1 px-4 py-2 rounded-lg text-sm font-medium bg-subtle hover:bg-hover text-secondary transition-colors"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={() => {
                  setRunAllConfirmOpen(false)
                  p.onRunAll()
                }}
                className="flex-1 px-4 py-2 rounded-lg text-sm font-medium bg-emerald-600 hover:bg-emerald-500 text-white transition-colors"
              >
                Queue all
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </div>
  )
}
