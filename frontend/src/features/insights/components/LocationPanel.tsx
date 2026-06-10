import type { CSSProperties } from 'react'
import type { LocationData } from '@/types'
import { InsightCard, InsightEmpty, InsightError, InsightSkeleton } from './InsightCard'
import { useLocation } from '../hooks/useInsights'

function LocationContent({ data }: { data: LocationData }) {
  const countries = data.countries
  const states = data.states
  if (countries.length === 0 && states.length === 0) return <InsightEmpty />
  const maxC = countries[0]?.count ?? 1
  const maxS = states[0]?.count ?? 1
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
      <div>
        <p className="text-xs text-muted mb-3 uppercase tracking-wider">By Country</p>
        <div className="space-y-2">
          {countries.slice(0, 8).map(({ country, count }) => (
            <div key={country} className="flex items-center gap-3">
              <span className="w-20 shrink-0 text-xs text-secondary truncate">{country}</span>
              <div className="flex-1 bg-subtle rounded-full h-1.5 overflow-hidden">
                <div
                  style={{ '--bar-pct': `${(count / maxC) * 100}%` } as CSSProperties}
                  className="h-full bg-violet-500 rounded-full [width:var(--bar-pct)]"
                />
              </div>
              <span className="w-8 text-right text-xs text-muted shrink-0">{count}</span>
            </div>
          ))}
        </div>
      </div>
      <div>
        <p className="text-xs text-muted mb-3 uppercase tracking-wider">USA by State</p>
        {states.length === 0 ? (
          <p className="text-xs text-placeholder">No US state data yet</p>
        ) : (
          <div className="space-y-2">
            {states.slice(0, 8).map(({ state, count }) => (
              <div key={state} className="flex items-center gap-3">
                <span className="w-12 shrink-0 text-xs text-secondary">{state}</span>
                <div className="flex-1 bg-subtle rounded-full h-1.5 overflow-hidden">
                  <div
                    style={{ '--bar-pct': `${(count / maxS) * 100}%` } as CSSProperties}
                    className="h-full bg-sky-500 rounded-full [width:var(--bar-pct)]"
                  />
                </div>
                <span className="w-8 text-right text-xs text-muted shrink-0">{count}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

export function LocationPanel() {
  const { data, isLoading, isError } = useLocation()
  return (
    <InsightCard
      title="Location Breakdown"
      subtitle="Where jobs are located (structured, extracted by LLM)"
    >
      {isLoading ? (
        <InsightSkeleton />
      ) : isError || !data ? (
        <InsightError />
      ) : (
        <LocationContent data={data} />
      )}
    </InsightCard>
  )
}
