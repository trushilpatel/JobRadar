import { useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { AlertCircle, RefreshCw, CheckCircle2 } from 'lucide-react'
import { cn } from '@/lib/utils/utils'
import { fetchPipelineStats, triggerRetry } from '@/lib/api/pipeline'
import type { RetryTierCounts } from '@/types'
import { ErrorBoundary } from '@/components/shared/ErrorBoundary'
import { AtsTable } from '@/features/settings'
import { useAts, useAtsActions } from '@/features/ats'

const PIPELINE_QUERY_KEY = ['admin', 'pipeline-stats'] as const
const POLL_INTERVAL_MS = 30_000

// ── Pipeline metric card ──────────────────────────────────────────────────────

interface MetricCardProps {
  label: string
  value: number
  variant?: 'neutral' | 'green' | 'amber' | 'red' | 'blue' | 'violet'
  dim?: boolean
}

const VARIANT_CLASSES: Record<NonNullable<MetricCardProps['variant']>, string> = {
  neutral: 'text-primary',
  green: 'text-emerald-500 dark:text-emerald-400',
  amber: 'text-amber-500 dark:text-amber-400',
  red: 'text-red-500 dark:text-red-400',
  blue: 'text-blue-500 dark:text-blue-400',
  violet: 'text-violet-500 dark:text-violet-400',
}

function MetricCard({ label, value, variant = 'neutral', dim }: MetricCardProps) {
  const colorClass = dim || value === 0 ? 'text-placeholder' : VARIANT_CLASSES[variant]
  return (
    <div className="flex flex-col gap-1 p-3 rounded-lg bg-subtle border border-[var(--border)]">
      <span className={cn('text-2xl font-semibold tabular-nums leading-none', colorClass)}>
        {value.toLocaleString()}
      </span>
      <span className="text-xs text-muted leading-tight">{label}</span>
    </div>
  )
}

// ── Retry tier breakdown ──────────────────────────────────────────────────────

interface RetryBreakdownProps {
  label: string
  worker: 'filter' | 'enricher'
  counts: RetryTierCounts
}

const TIERS: Array<{ key: keyof RetryTierCounts; label: string }> = [
  { key: '1m', label: '1 min' },
  { key: '10m', label: '10 min' },
  { key: '1h', label: '1 hour' },
  { key: 'dlq', label: 'DLQ' },
]

function RetryBreakdown({ label, worker, counts }: RetryBreakdownProps) {
  const qc = useQueryClient()
  const { mutate, isPending } = useMutation({
    mutationFn: ({ tier }: { tier: keyof RetryTierCounts }) => triggerRetry(worker, tier),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: PIPELINE_QUERY_KEY })
    },
  })
  const total = counts['1m'] + counts['10m'] + counts['1h'] + counts.dlq

  return (
    <div className="flex flex-col gap-2 p-3 rounded-lg bg-subtle border border-[var(--border)]">
      <div className="flex items-baseline justify-between">
        <span className="text-xs font-medium text-secondary">{label}</span>
        <span
          className={cn(
            'text-sm font-semibold tabular-nums',
            total === 0 ? 'text-placeholder' : 'text-amber-500 dark:text-amber-400'
          )}
        >
          {total}
        </span>
      </div>
      <div className="space-y-1">
        {TIERS.map(({ key, label: tierLabel }) => (
          <div key={key} className="flex items-center gap-2">
            <span className="text-xs text-muted w-12 shrink-0">{tierLabel}</span>
            <span className="text-xs tabular-nums text-secondary flex-1 text-right">
              {counts[key]}
            </span>
            <button
              type="button"
              disabled={counts[key] === 0 || isPending}
              onClick={() => mutate({ tier: key })}
              title={
                key === 'dlq'
                  ? 'Recovery operation: re-queues permanently failed jobs. Use with care.'
                  : undefined
              }
              className={cn(
                'text-[10px] px-1.5 py-0.5 rounded border transition-colors shrink-0',
                counts[key] === 0 || isPending
                  ? 'border-[var(--border)] text-placeholder cursor-not-allowed'
                  : key === 'dlq'
                    ? 'border-amber-500/50 text-amber-500 dark:text-amber-400 hover:bg-amber-500/10 cursor-pointer'
                    : 'border-violet-500/50 text-violet-500 dark:text-violet-400 hover:bg-violet-500/10 cursor-pointer'
              )}
            >
              {key === 'dlq' ? 'Retry ⚠' : 'Retry'}
            </button>
          </div>
        ))}
      </div>
    </div>
  )
}

// ── Pipeline health panel ─────────────────────────────────────────────────────

function PipelineHealthSkeleton() {
  return (
    <div className="space-y-4 animate-pulse">
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
        {Array.from({ length: 5 }, (_, i) => (
          <div key={i} className="h-[72px] rounded-lg bg-subtle border border-[var(--border)]" />
        ))}
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {Array.from({ length: 2 }, (_, i) => (
          <div key={i} className="h-[120px] rounded-lg bg-subtle border border-[var(--border)]" />
        ))}
      </div>
    </div>
  )
}

function PipelineHealthPanel() {
  const { data, isLoading, isError, dataUpdatedAt, refetch } = useQuery({
    queryKey: PIPELINE_QUERY_KEY,
    queryFn: fetchPipelineStats,
    refetchInterval: POLL_INTERVAL_MS,
    staleTime: POLL_INTERVAL_MS,
  })

  const lastUpdated = useMemo(() => {
    if (!dataUpdatedAt) return null
    return new Date(dataUpdatedAt).toLocaleTimeString([], {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  }, [dataUpdatedAt])

  if (isLoading) return <PipelineHealthSkeleton />

  if (isError || !data) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted py-4">
        <AlertCircle size={14} className="text-red-400 shrink-0" />
        Pipeline stats unavailable — backend may be unreachable
      </div>
    )
  }

  const hasErrors = data.errors_24h > 0
  const totalRetries =
    data.filter_retries['1m'] +
    data.filter_retries['10m'] +
    data.filter_retries['1h'] +
    data.filter_retries.dlq +
    data.enricher_retries['1m'] +
    data.enricher_retries['10m'] +
    data.enricher_retries['1h'] +
    data.enricher_retries.dlq

  return (
    <div className="space-y-4">
      {/* Status bar */}
      <div className="flex items-center justify-between text-xs text-muted">
        <div className="flex items-center gap-1.5">
          {!hasErrors && totalRetries === 0 ? (
            <CheckCircle2 size={12} className="text-emerald-500" />
          ) : (
            <AlertCircle size={12} className="text-amber-500" />
          )}
          <span>
            {!hasErrors && totalRetries === 0
              ? 'Pipeline healthy'
              : `${hasErrors ? `${data.errors_24h} error${data.errors_24h !== 1 ? 's' : ''} in 24h` : ''}${hasErrors && totalRetries > 0 ? ' · ' : ''}${totalRetries > 0 ? `${totalRetries} retrying` : ''}`}
          </span>
        </div>
        {lastUpdated && (
          <button
            type="button"
            onClick={() => void refetch()}
            className="flex items-center gap-1 hover:text-primary transition-colors cursor-pointer"
            title="Click to refresh"
          >
            <RefreshCw size={10} />
            <span>Last updated {lastUpdated} · every 30s</span>
          </button>
        )}
      </div>

      {/* Stage metrics */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
        <MetricCard label="Active companies" value={data.active_companies} variant="green" />
        <MetricCard label="Queued" value={data.queued} variant="amber" />
        <MetricCard label="Crawling" value={data.crawling} variant="blue" />
        <MetricCard label="Awaiting filter" value={data.awaiting_filter} variant="violet" />
        <MetricCard label="Awaiting enrichment" value={data.awaiting_enrichment} variant="violet" />
      </div>

      {/* Output metrics */}
      <div className="grid grid-cols-2 gap-3">
        <MetricCard label="New jobs (24h)" value={data.new_jobs_24h} variant="green" />
        <MetricCard
          label="Errors (24h)"
          value={data.errors_24h}
          variant={data.errors_24h > 0 ? 'red' : 'neutral'}
        />
      </div>

      {/* Retry controls */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <RetryBreakdown label="Filter retries" worker="filter" counts={data.filter_retries} />
        <RetryBreakdown label="Enricher retries" worker="enricher" counts={data.enricher_retries} />
      </div>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

export function AdminPipelinePage() {
  const { data: atsList, isLoading, isError, refetch } = useAts()
  const mutation = useAtsActions()

  return (
    <div className="flex-1 overflow-y-auto p-6 max-w-6xl mx-auto w-full space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-primary">Pipeline</h1>
        <p className="text-sm text-muted mt-1">
          Real-time pipeline health and per-adapter scan configuration.
        </p>
      </div>

      <section>
        <div className="mb-4">
          <h2 className="text-sm font-semibold text-primary">Pipeline Health</h2>
        </div>
        <ErrorBoundary>
          <PipelineHealthPanel />
        </ErrorBoundary>
      </section>

      <section>
        <div className="mb-4">
          <h2 className="text-sm font-semibold text-primary">Adapter Configuration</h2>
          <p className="text-xs text-muted mt-1">
            Per-adapter scan intervals and inter-company delays. Changes apply to all companies
            using that adapter unless a company-level override is set.
          </p>
        </div>
        <ErrorBoundary>
          <AtsTable
            atsList={atsList}
            isLoading={isLoading}
            isError={isError}
            refetch={refetch}
            onUpdate={(name, data) => mutation.mutateAsync({ name, data })}
          />
        </ErrorBoundary>
      </section>
    </div>
  )
}
