import { z } from 'zod'
import type {
  ProgressStats,
  MarketStats,
  WeeklyCount,
  InterviewRow,
  CompanyVelocityRow,
  TechTrends,
} from '@/types'

import { apiClient } from '@/lib/core/api-client'
export { fetchPipelineStats, triggerRetry } from '@/lib/api/pipeline'

// ── Zod schemas ──────────────────────────────────────────────────────────────

const ProgressStatsSchema = z
  .object({
    applied_count: z.number().optional().default(0),
    awaiting_reply: z.number().optional().default(0),
    interviewing_count: z.number().optional().default(0),
    interviewing_this_week: z.number().optional().default(0),
    offer_count: z.number().optional().default(0),
  })
  .nullable()

const MarketStatsSchema = z
  .object({
    new_since_yesterday: z.number().optional().default(0),
    total_open: z.number().optional().default(0),
    added_this_week: z.number().optional().default(0),
    added_last_week: z.number().optional().default(0),
    weekly_delta: z.number().optional().default(0),
  })
  .nullable()

const WeeklyCountSchema = z.object({
  week_start: z.string(),
  job_count: z.number(),
})

const HeatmapResponseSchema = z.array(WeeklyCountSchema).nullable()

const InterviewRowSchema = z.object({
  application_id: z.number(),
  job_id: z.string(),
  company_name: z.string(),
  job_title: z.string(),
  stage: z.string(),
  display_stage: z.string(),
  stage_at: z.string(),
})

const InterviewsResponseSchema = z.array(InterviewRowSchema).nullable()

const CompanyVelocityRowSchema = z.object({
  company_id: z.string(),
  company_name: z.string(),
  week_current: z.number(),
  week_1: z.number(),
  week_2: z.number(),
  week_3: z.number(),
  multiplier: z.number(),
  velocity_tag: z.enum(['spike', 'rising', 'normal']),
})

const VelocityResponseSchema = z.array(CompanyVelocityRowSchema).nullable()

const TechTrendRowSchema = z.object({
  tech: z.string(),
  m1: z.number(),
  m2: z.number(),
  m3: z.number(),
  m4: z.number(),
  growth_pct: z.number(),
  adoption_pct: z.number(),
  is_new: z.boolean(),
})

const TechTrendsSchema = z
  .object({
    total_jobs: z.number().optional().default(0),
    rising: z
      .array(TechTrendRowSchema)
      .nullish()
      .transform((v) => v ?? []),
    new_entrants: z
      .array(TechTrendRowSchema)
      .nullish()
      .transform((v) => v ?? []),
  })
  .nullable()

// ── API functions ────────────────────────────────────────────────────────────

export async function fetchProgress(): Promise<ProgressStats> {
  const raw = await apiClient.get<unknown>('/dashboard/progress')
  const data = ProgressStatsSchema.parse(raw)
  return {
    applied_count: data?.applied_count ?? 0,
    awaiting_reply: data?.awaiting_reply ?? 0,
    interviewing_count: data?.interviewing_count ?? 0,
    interviewing_this_week: data?.interviewing_this_week ?? 0,
    offer_count: data?.offer_count ?? 0,
  }
}

export async function fetchMarket(): Promise<MarketStats> {
  const raw = await apiClient.get<unknown>('/dashboard/market')
  const data = MarketStatsSchema.parse(raw)
  return {
    new_since_yesterday: data?.new_since_yesterday ?? 0,
    total_open: data?.total_open ?? 0,
    added_this_week: data?.added_this_week ?? 0,
    added_last_week: data?.added_last_week ?? 0,
    weekly_delta: data?.weekly_delta ?? 0,
  }
}

export async function fetchHeatmap(): Promise<WeeklyCount[]> {
  const raw = await apiClient.get<unknown>('/dashboard/heatmap')
  const data = HeatmapResponseSchema.parse(raw)
  return data ?? []
}

export async function fetchInterviews(): Promise<InterviewRow[]> {
  const raw = await apiClient.get<unknown>('/dashboard/interviews')
  const data = InterviewsResponseSchema.parse(raw)
  return data ?? []
}

export async function fetchVelocity(limit = 10): Promise<CompanyVelocityRow[]> {
  const raw = await apiClient.get<unknown>(`/dashboard/velocity?limit=${limit}`)
  const data = VelocityResponseSchema.parse(raw)
  return data ?? []
}

export async function fetchTechTrends(): Promise<TechTrends> {
  const raw = await apiClient.get<unknown>('/dashboard/tech-trends')
  const data = TechTrendsSchema.parse(raw)
  return {
    total_jobs: data?.total_jobs ?? 0,
    rising: data?.rising ?? [],
    new_entrants: data?.new_entrants ?? [],
  }
}

export async function updateApplicationStage(id: number, stage: string): Promise<void> {
  await apiClient.patch<void>(`/applications/${id}/stage`, { stage })
}
