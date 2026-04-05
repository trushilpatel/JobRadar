import { useEffect, useState, useMemo } from 'react'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  Cell,
} from 'recharts'
import { fetchTechStack, fetchCompanyActivity } from '@/api/insights'
import type { TechCount, CompanyWeekCount } from '@/api/insights'

// ─── InsightCard ──────────────────────────────────────────────────────────────

function InsightCard({
  title,
  subtitle,
  children,
}: {
  title: string
  subtitle: string
  children: React.ReactNode
}) {
  return (
    <div className="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
      <div className="mb-4">
        <h2 className="text-sm font-semibold text-white">{title}</h2>
        <p className="text-xs text-slate-500 mt-0.5">{subtitle}</p>
      </div>
      {children}
    </div>
  )
}

// ─── Spinner ──────────────────────────────────────────────────────────────────

function Spinner() {
  return (
    <div className="flex items-center justify-center h-32">
      <div className="w-6 h-6 border-2 border-violet-500 border-t-transparent rounded-full animate-spin" />
    </div>
  )
}

// ─── Tech Stack Chart ─────────────────────────────────────────────────────────

function TechStackChart({ data }: { data: TechCount[] }) {
  if (data.length === 0) {
    return (
      <div className="flex items-center justify-center h-32 text-slate-500 text-sm">
        Not enough data yet
      </div>
    )
  }

  return (
    <ResponsiveContainer width="100%" height={420}>
      <BarChart
        layout="vertical"
        data={data}
        margin={{ left: 10, right: 24, top: 4, bottom: 4 }}
      >
        <XAxis
          type="number"
          stroke="#334155"
          tick={{ fill: '#94a3b8', fontSize: 11 }}
          tickLine={false}
          axisLine={false}
        />
        <YAxis
          type="category"
          dataKey="tech"
          stroke="#334155"
          tick={{ fill: '#94a3b8', fontSize: 12 }}
          width={90}
          tickLine={false}
          axisLine={false}
        />
        <Tooltip
          cursor={{ fill: 'rgba(255,255,255,0.03)' }}
          contentStyle={{
            background: '#13131e',
            border: '1px solid rgba(255,255,255,0.07)',
            borderRadius: 8,
            color: '#e2e8f0',
            fontSize: 12,
          }}
          formatter={(value: number) => [value, 'Jobs']}
        />
        <Bar dataKey="count" radius={[0, 4, 4, 0]} maxBarSize={22}>
          {data.map((_, i) => (
            <Cell
              key={i}
              fill={i === 0 ? '#7c3aed' : i < 5 ? '#6d28d9' : '#4c1d95'}
            />
          ))}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  )
}

// ─── Company Activity Table ───────────────────────────────────────────────────

interface CompanyStat {
  company_name: string
  total: number
}

function CompanyActivityTable({ data }: { data: CompanyWeekCount[] }) {
  const stats = useMemo<CompanyStat[]>(() => {
    const map: Record<string, number> = {}
    for (const row of data) {
      map[row.company_name] = (map[row.company_name] ?? 0) + row.count
    }
    return Object.entries(map)
      .map(([company_name, total]) => ({ company_name, total }))
      .sort((a, b) => b.total - a.total)
  }, [data])

  if (stats.length === 0) {
    return (
      <div className="flex items-center justify-center h-32 text-slate-500 text-sm">
        Not enough data yet
      </div>
    )
  }

  const max = stats[0]?.total ?? 1

  return (
    <div className="space-y-2">
      {stats.map(({ company_name, total }) => (
        <div key={company_name} className="flex items-center gap-3">
          <p className="w-40 shrink-0 text-sm text-slate-300 truncate" title={company_name}>
            {company_name}
          </p>
          <div className="flex-1 bg-white/[0.04] rounded-full h-2 overflow-hidden">
            <div
              className="h-full bg-violet-600 rounded-full transition-all"
              style={{ width: `${(total / max) * 100}%` }}
            />
          </div>
          <span className="w-8 text-right text-xs text-slate-400 shrink-0">{total}</span>
        </div>
      ))}
    </div>
  )
}

// ─── Main Page ────────────────────────────────────────────────────────────────

export function InsightsPage() {
  const [techData, setTechData] = useState<TechCount[]>([])
  const [activityData, setActivityData] = useState<CompanyWeekCount[]>([])
  const [loadingTech, setLoadingTech] = useState(true)
  const [loadingActivity, setLoadingActivity] = useState(true)

  useEffect(() => {
    setLoadingTech(true)
    fetchTechStack()
      .then(d => setTechData((d ?? []).slice(0, 15)))
      .catch(() => setTechData([]))
      .finally(() => setLoadingTech(false))

    setLoadingActivity(true)
    fetchCompanyActivity()
      .then(d => setActivityData(d ?? []))
      .catch(() => setActivityData([]))
      .finally(() => setLoadingActivity(false))
  }, [])

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Header */}
      <div className="px-6 py-5 border-b border-white/[0.07]">
        <h1 className="text-xl font-semibold text-white">Insights</h1>
        <p className="text-sm text-slate-400 mt-0.5">Based on your job pipeline data</p>
      </div>

      {/* Cards */}
      <div className="flex-1 overflow-auto px-6 py-5 space-y-5">
        <InsightCard
          title="Tech Stack Trends"
          subtitle="Top 15 technologies across all scanned jobs"
        >
          {loadingTech ? <Spinner /> : <TechStackChart data={techData} />}
        </InsightCard>

        <InsightCard
          title="Company Activity"
          subtitle="Jobs posted per company (all time)"
        >
          {loadingActivity ? <Spinner /> : <CompanyActivityTable data={activityData} />}
        </InsightCard>
      </div>
    </div>
  )
}
