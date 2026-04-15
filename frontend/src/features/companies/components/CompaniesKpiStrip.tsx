import { KpiCard } from '@/components/ui/kpi-card'

interface CompaniesKpiStripProps {
  total: number
  totalActive: number
  running: number
  queued: number
  errors: number
  errorFilterActive: boolean
  onErrorToggle: () => void
}

export function CompaniesKpiStrip({
  total, totalActive, running, queued, errors,
  errorFilterActive, onErrorToggle,
}: CompaniesKpiStripProps) {
  return (
    <div className="grid grid-cols-5 gap-2 px-6 py-3 border-b border-white/[0.07]">
      <KpiCard value={total} label="Tracked" />
      <KpiCard value={totalActive} label="Active" valueClassName="text-emerald-400" />
      <KpiCard value={running} label="Running" valueClassName="text-blue-400" />
      <KpiCard value={queued} label="In queue" valueClassName="text-amber-400" />
      <KpiCard
        value={errors}
        label="Errors · click to filter"
        valueClassName="text-red-400"
        clickable
        active={errorFilterActive}
        onClick={onErrorToggle}
        title="Toggle error filter"
      />
    </div>
  )
}
