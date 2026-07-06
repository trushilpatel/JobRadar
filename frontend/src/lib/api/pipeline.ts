// Shared pipeline API — used by both the dashboard and companies features.
// Lives in lib/ so any feature can import it without triggering cross-feature
// boundary violations.
import { z } from 'zod'
import type { PipelineStats, RetryTierCounts } from '@/types'
import { apiClient } from '@/lib/core/api-client'
import type { components } from '@/lib/api/schema.gen'

const RetryTierCountsSchema = z.object({
  '1m': z.number().default(0),
  '10m': z.number().default(0),
  '1h': z.number().default(0),
  dlq: z.number().default(0),
})

const PipelineStatsSchema = z
  .object({
    total_companies: z.number().optional().default(0),
    active_companies: z.number().optional().default(0),
    queued: z.number().optional().default(0),
    crawling: z.number().optional().default(0),
    awaiting_filter: z.number().optional().default(0),
    filter_retries: RetryTierCountsSchema.optional(),
    awaiting_enrichment: z.number().optional().default(0),
    enricher_retries: RetryTierCountsSchema.optional(),
    new_jobs_24h: z.number().optional().default(0),
    errors_24h: z.number().optional().default(0),
  })
  .nullable()

const DrainedSchema = z.object({ drained: z.number() })

export async function fetchPipelineStats(): Promise<PipelineStats> {
  const raw = await apiClient.get<unknown>('/admin/pipeline/stats')
  const data = PipelineStatsSchema.parse(raw)
  const emptyRetries: RetryTierCounts = { '1m': 0, '10m': 0, '1h': 0, dlq: 0 }
  return {
    total_companies: data?.total_companies ?? 0,
    active_companies: data?.active_companies ?? 0,
    queued: data?.queued ?? 0,
    crawling: data?.crawling ?? 0,
    awaiting_filter: data?.awaiting_filter ?? 0,
    filter_retries: data?.filter_retries ?? emptyRetries,
    awaiting_enrichment: data?.awaiting_enrichment ?? 0,
    enricher_retries: data?.enricher_retries ?? emptyRetries,
    new_jobs_24h: data?.new_jobs_24h ?? 0,
    errors_24h: data?.errors_24h ?? 0,
  }
}

export async function triggerRetry(
  worker: 'filter' | 'enricher',
  tier: '1m' | '10m' | '1h' | 'dlq'
): Promise<{ drained: number }> {
  // `satisfies` cross-checks this request body against the backend's
  // generated OpenAPI contract at compile time.
  const body = { worker, tier } satisfies components['schemas']['TriggerRetryInput']
  const raw = await apiClient.post<unknown>('/admin/pipeline/retry', body)
  return DrainedSchema.parse(raw)
}
