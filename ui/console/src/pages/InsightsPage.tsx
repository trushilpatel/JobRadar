import { PipelineFunnelPanel } from '@/features/insights/PipelineFunnelPanel'
import { RoleCategoryPanel } from '@/features/insights/RoleCategoryPanel'
import { CompanyVelocityPanel } from '@/features/insights/CompanyVelocityPanel'
import { LocationPanel } from '@/features/insights/LocationPanel'
import { SponsorshipTrendPanel } from '@/features/insights/SponsorshipTrendPanel'
import { TechStackPanel } from '@/features/insights/TechStackPanel'
import { CompanyActivityPanel } from '@/features/insights/CompanyActivityPanel'

export function InsightsPage() {
  return (
    <div className="flex-1 flex flex-col min-h-0">
      <div className="px-6 py-5 border-b border-white/[0.07]">
        <h1 className="text-xl font-semibold text-white">Insights</h1>
        <p className="text-sm text-slate-400 mt-0.5">Pipeline analytics — based on all enriched jobs</p>
      </div>

      <div className="flex-1 overflow-auto px-6 py-5 space-y-5">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
          <PipelineFunnelPanel />
          <RoleCategoryPanel />
        </div>
        <CompanyVelocityPanel />
        <LocationPanel />
        <SponsorshipTrendPanel />
        <TechStackPanel />
        <CompanyActivityPanel />
      </div>
    </div>
  )
}
