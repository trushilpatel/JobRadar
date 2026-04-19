import { ProgressPanel } from '@/features/dashboard/ProgressPanel'
import { MarketPanel } from '@/features/dashboard/MarketPanel'
import { HeatmapPanel } from '@/features/dashboard/HeatmapPanel'
import { InterviewingPanel } from '@/features/dashboard/InterviewingPanel'
import { VelocityPanel } from '@/features/dashboard/VelocityPanel'
import { TechTrendsPanel } from '@/features/dashboard/TechTrendsPanel'

export function DashboardPage() {
  return (
    <div className="flex-1 flex flex-col min-h-0 overflow-auto">
      <div className="flex-1 px-6 py-6 space-y-4">
        {/* Row 1: My Progress + Market */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <ProgressPanel />
          <MarketPanel />
        </div>

        {/* Row 2: Hiring Activity Heatmap */}
        <HeatmapPanel />

        {/* Row 3: Interviewing + Velocity */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <InterviewingPanel />
          <VelocityPanel />
        </div>

        {/* Row 4: Tech Stack Trends */}
        <TechTrendsPanel />
      </div>
    </div>
  )
}
