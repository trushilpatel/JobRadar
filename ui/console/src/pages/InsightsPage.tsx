import { useEffect, useState, useMemo } from 'react'
import {
  BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell,
  LineChart, Line, CartesianGrid,
} from 'recharts'
import {
  fetchTechStack, fetchCompanyActivity, fetchFunnel,
  fetchLocation, fetchSponsorshipTrend, fetchVelocity, fetchRoleCategories,
} from '@/api/insights'
import type {
  TechCount, CompanyWeekCount, FunnelStep, LocationData,
  SponsorshipDay, CompanyVelocity, RoleCategoryCount,
} from '@/api/insights'
import { TrendingUp, TrendingDown, Minus } from 'lucide-react'
import { cn } from '@/lib/utils'

// ─── Shared helpers ───────────────────────────────────────────────────────────

function InsightCard({ title, subtitle, children }: {
  title: string; subtitle: string; children: React.ReactNode
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

function Empty() {
  return (
    <div className="flex items-center justify-center h-32 text-slate-500 text-sm">
      Not enough data yet
    </div>
  )
}

const tooltipStyle = {
  background: '#13131e',
  border: '1px solid rgba(255,255,255,0.07)',
  borderRadius: 8,
  color: '#e2e8f0',
  fontSize: 12,
}

// ─── Pipeline Funnel ──────────────────────────────────────────────────────────

function PipelineFunnel({ data }: { data: FunnelStep[] }) {
  if (data.length === 0) return <Empty />

  const order = ['enriched', 'saved', 'applied', 'dismissed']
  const colors: Record<string, string> = {
    enriched: 'bg-slate-500',
    saved: 'bg-amber-500',
    applied: 'bg-emerald-500',
    dismissed: 'bg-red-500/60',
  }
  const labels: Record<string, string> = {
    enriched: 'Enriched (total)',
    saved: 'Saved',
    applied: 'Applied',
    dismissed: 'Dismissed',
  }

  const map = Object.fromEntries(data.map(d => [d.stage, d.count]))
  const total = map['enriched'] ?? 1

  return (
    <div className="space-y-3">
      {order.map(stage => {
        const count = map[stage] ?? 0
        const pct = Math.round((count / total) * 100)
        return (
          <div key={stage}>
            <div className="flex items-center justify-between mb-1.5">
              <span className="text-xs text-slate-400">{labels[stage]}</span>
              <span className="text-xs font-medium text-white tabular-nums">
                {count}
                <span className="text-slate-500 ml-1.5">({pct}%)</span>
              </span>
            </div>
            <div className="h-1.5 bg-white/[0.06] rounded-full overflow-hidden">
              <div
                className={cn('h-full rounded-full transition-all', colors[stage])}
                style={{ width: `${Math.max(pct, count > 0 ? 2 : 0)}%` }}
              />
            </div>
          </div>
        )
      })}
    </div>
  )
}

// ─── Location Breakdown ───────────────────────────────────────────────────────

function LocationBreakdown({ data }: { data: LocationData }) {
  const countries = data?.countries ?? []
  const states = data?.states ?? []
  if (countries.length === 0 && states.length === 0) return <Empty />

  const maxC = countries[0]?.count ?? 1
  const maxS = states[0]?.count ?? 1

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
      {/* Countries */}
      <div>
        <p className="text-xs text-slate-500 mb-3 uppercase tracking-wider">By Country</p>
        <div className="space-y-2">
          {countries.slice(0, 8).map(({ country, count }) => (
            <div key={country} className="flex items-center gap-3">
              <span className="w-20 shrink-0 text-xs text-slate-300 truncate">{country}</span>
              <div className="flex-1 bg-white/[0.04] rounded-full h-1.5 overflow-hidden">
                <div className="h-full bg-violet-500 rounded-full" style={{ width: `${(count / maxC) * 100}%` }} />
              </div>
              <span className="w-8 text-right text-xs text-slate-400 shrink-0">{count}</span>
            </div>
          ))}
        </div>
      </div>
      {/* US States */}
      <div>
        <p className="text-xs text-slate-500 mb-3 uppercase tracking-wider">USA by State</p>
        {states.length === 0 ? (
          <p className="text-xs text-slate-600">No US state data yet</p>
        ) : (
          <div className="space-y-2">
            {states.slice(0, 8).map(({ state, count }) => (
              <div key={state} className="flex items-center gap-3">
                <span className="w-12 shrink-0 text-xs text-slate-300">{state}</span>
                <div className="flex-1 bg-white/[0.04] rounded-full h-1.5 overflow-hidden">
                  <div className="h-full bg-sky-500 rounded-full" style={{ width: `${(count / maxS) * 100}%` }} />
                </div>
                <span className="w-8 text-right text-xs text-slate-400 shrink-0">{count}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// ─── Sponsorship Trend ────────────────────────────────────────────────────────

function SponsorshipTrend({ data }: { data: SponsorshipDay[] }) {
  if (data.length === 0) return <Empty />

  const chartData = data.map(d => ({
    date: d.date.slice(5), // MM-DD
    total: d.total,
    sponsored: d.sponsored,
    pct: d.total > 0 ? Math.round((d.sponsored / d.total) * 100) : 0,
  }))

  return (
    <ResponsiveContainer width="100%" height={180}>
      <LineChart data={chartData} margin={{ left: 0, right: 16, top: 4, bottom: 4 }}>
        <CartesianGrid stroke="rgba(255,255,255,0.04)" vertical={false} />
        <XAxis dataKey="date" tick={{ fill: '#64748b', fontSize: 11 }} tickLine={false} axisLine={false} />
        <YAxis tick={{ fill: '#64748b', fontSize: 11 }} tickLine={false} axisLine={false} width={30} />
        <Tooltip contentStyle={tooltipStyle} formatter={(v) => [v, '']} />
        <Line type="monotone" dataKey="total" stroke="#64748b" dot={false} strokeWidth={1.5} name="Total" />
        <Line type="monotone" dataKey="sponsored" stroke="#7c3aed" dot={false} strokeWidth={2} name="Sponsored" />
      </LineChart>
    </ResponsiveContainer>
  )
}

// ─── Company Velocity ─────────────────────────────────────────────────────────

function CompanyVelocityTable({ data }: { data: CompanyVelocity[] }) {
  if (data.length === 0) return <Empty />

  return (
    <div className="overflow-x-auto">
      <table className="w-full">
        <thead>
          <tr className="border-b border-white/[0.06]">
            <th className="py-2 px-3 text-left text-xs text-slate-500 font-medium">Company</th>
            <th className="py-2 px-3 text-center text-xs text-slate-500 font-medium">Last wk</th>
            <th className="py-2 px-3 text-center text-xs text-slate-500 font-medium">This wk</th>
            <th className="py-2 px-3 text-center text-xs text-slate-500 font-medium">Trend</th>
          </tr>
        </thead>
        <tbody>
          {data.map(row => (
            <tr key={row.company_id} className="border-b border-white/[0.03] hover:bg-white/[0.02]">
              <td className="py-2.5 px-3 text-sm text-slate-200">{row.company_name}</td>
              <td className="py-2.5 px-3 text-center text-sm text-slate-400 tabular-nums">{row.last_week}</td>
              <td className="py-2.5 px-3 text-center text-sm text-white tabular-nums font-medium">{row.this_week}</td>
              <td className="py-2.5 px-3">
                <div className="flex items-center justify-center gap-1">
                  {row.delta > 0 ? (
                    <span className="flex items-center gap-1 text-xs text-emerald-400 font-medium">
                      <TrendingUp size={12} />+{row.delta}
                    </span>
                  ) : row.delta < 0 ? (
                    <span className="flex items-center gap-1 text-xs text-red-400">
                      <TrendingDown size={12} />{row.delta}
                    </span>
                  ) : (
                    <span className="flex items-center gap-1 text-xs text-slate-500">
                      <Minus size={12} />0
                    </span>
                  )}
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ─── Tech Stack Chart ─────────────────────────────────────────────────────────

function TechStackChart({ data }: { data: TechCount[] }) {
  if (data.length === 0) return <Empty />
  return (
    <ResponsiveContainer width="100%" height={420}>
      <BarChart layout="vertical" data={data} margin={{ left: 10, right: 24, top: 4, bottom: 4 }}>
        <XAxis type="number" stroke="#334155" tick={{ fill: '#94a3b8', fontSize: 11 }} tickLine={false} axisLine={false} />
        <YAxis type="category" dataKey="tech" stroke="#334155" tick={{ fill: '#94a3b8', fontSize: 12 }} width={90} tickLine={false} axisLine={false} />
        <Tooltip cursor={{ fill: 'rgba(255,255,255,0.03)' }} contentStyle={tooltipStyle} formatter={(value) => [value ?? 0, 'Jobs']} />
        <Bar dataKey="count" radius={[0, 4, 4, 0]} maxBarSize={22}>
          {data.map((_, i) => (
            <Cell key={i} fill={i === 0 ? '#7c3aed' : i < 5 ? '#6d28d9' : '#4c1d95'} />
          ))}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  )
}

// ─── Role Category Chart ──────────────────────────────────────────────────────

const ROLE_COLORS: Record<string, string> = {
  backend: '#7c3aed',
  frontend: '#2563eb',
  fullstack: '#0891b2',
  devops: '#059669',
  ml: '#d97706',
  data: '#dc2626',
  other: '#475569',
}

function RoleCategoryChart({ data }: { data: RoleCategoryCount[] }) {
  if (data.length === 0) return <Empty />
  const max = data[0]?.count ?? 1
  return (
    <div className="space-y-2.5">
      {data.map(({ category, count }) => (
        <div key={category} className="flex items-center gap-3">
          <span className="w-20 shrink-0 text-xs text-slate-300 capitalize">{category}</span>
          <div className="flex-1 bg-white/[0.04] rounded-full h-2 overflow-hidden">
            <div
              className="h-full rounded-full transition-all"
              style={{
                width: `${(count / max) * 100}%`,
                backgroundColor: ROLE_COLORS[category] ?? '#475569',
              }}
            />
          </div>
          <span className="w-8 text-right text-xs text-slate-400 shrink-0">{count}</span>
        </div>
      ))}
    </div>
  )
}

// ─── Company Activity ─────────────────────────────────────────────────────────

function CompanyActivityTable({ data }: { data: CompanyWeekCount[] }) {
  const stats = useMemo(() => {
    const map: Record<string, number> = {}
    for (const row of data) {
      map[row.company_name] = (map[row.company_name] ?? 0) + row.count
    }
    return Object.entries(map)
      .map(([company_name, total]) => ({ company_name, total }))
      .sort((a, b) => b.total - a.total)
  }, [data])

  if (stats.length === 0) return <Empty />
  const max = stats[0]?.total ?? 1

  return (
    <div className="space-y-2">
      {stats.map(({ company_name, total }) => (
        <div key={company_name} className="flex items-center gap-3">
          <p className="w-40 shrink-0 text-sm text-slate-300 truncate" title={company_name}>{company_name}</p>
          <div className="flex-1 bg-white/[0.04] rounded-full h-2 overflow-hidden">
            <div className="h-full bg-violet-600 rounded-full transition-all" style={{ width: `${(total / max) * 100}%` }} />
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
  const [funnelData, setFunnelData] = useState<FunnelStep[]>([])
  const [locationData, setLocationData] = useState<LocationData>({ countries: [], states: [] })
  const [sponsorshipData, setSponsorshipData] = useState<SponsorshipDay[]>([])
  const [velocityData, setVelocityData] = useState<CompanyVelocity[]>([])
  const [roleData, setRoleData] = useState<RoleCategoryCount[]>([])

  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.allSettled([
      fetchTechStack(15).then(d => setTechData(d ?? [])),
      fetchCompanyActivity().then(d => setActivityData(d ?? [])),
      fetchFunnel().then(d => setFunnelData(d ?? [])),
      fetchLocation().then(d => setLocationData({ countries: d?.countries ?? [], states: d?.states ?? [] })),
      fetchSponsorshipTrend(30).then(d => setSponsorshipData(d ?? [])),
      fetchVelocity(20).then(d => setVelocityData(d ?? [])),
      fetchRoleCategories().then(d => setRoleData(d ?? [])),
    ]).finally(() => setLoading(false))
  }, [])

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Header */}
      <div className="px-6 py-5 border-b border-white/[0.07]">
        <h1 className="text-xl font-semibold text-white">Insights</h1>
        <p className="text-sm text-slate-400 mt-0.5">Pipeline analytics — based on all enriched jobs</p>
      </div>

      {loading ? (
        <div className="flex items-center justify-center flex-1">
          <div className="w-6 h-6 border-2 border-violet-500 border-t-transparent rounded-full animate-spin" />
        </div>
      ) : (
        <div className="flex-1 overflow-auto px-6 py-5 space-y-5">

          {/* Row 1: Funnel + Role Categories */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
            <InsightCard
              title="Pipeline Funnel"
              subtitle="Enriched → saved → applied (all time)"
            >
              <PipelineFunnel data={funnelData} />
            </InsightCard>

            <InsightCard
              title="Role Categories"
              subtitle="Distribution of enriched job types"
            >
              <RoleCategoryChart data={roleData} />
            </InsightCard>
          </div>

          {/* Row 2: Company Velocity */}
          <InsightCard
            title="Company Hiring Velocity"
            subtitle="This week vs last week — who just started or stopped hiring"
          >
            <CompanyVelocityTable data={velocityData} />
          </InsightCard>

          {/* Row 3: Location */}
          <InsightCard
            title="Location Breakdown"
            subtitle="Where jobs are located (structured, extracted by LLM)"
          >
            <LocationBreakdown data={locationData} />
          </InsightCard>

          {/* Row 4: Sponsorship trend */}
          <InsightCard
            title="Sponsorship Availability"
            subtitle="Daily: total enriched vs sponsorship-offering jobs (last 30 days)"
          >
            <SponsorshipTrend data={sponsorshipData} />
          </InsightCard>

          {/* Row 5: Tech Stack */}
          <InsightCard
            title="Tech Stack Trends"
            subtitle="Top 15 technologies across all enriched jobs"
          >
            <TechStackChart data={techData} />
          </InsightCard>

          {/* Row 6: Company Activity (all-time) */}
          <InsightCard
            title="Company Activity"
            subtitle="Total jobs enriched per company (all time)"
          >
            <CompanyActivityTable data={activityData} />
          </InsightCard>

        </div>
      )}
    </div>
  )
}
