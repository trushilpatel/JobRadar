export interface Company {
  id: string
  name: string
  url: string
  adapter_type: string
  active: boolean
  created_at: string
  last_scan_at: string | null
  last_scan_status: 'ok' | 'error' | 'pending' | null
  // Real scan history from company_scan_log: true=ok, false=error, oldest→newest.
  // Populated by the list endpoint. Absent for companies with no scan runs yet.
  scan_history?: boolean[]
}

export interface ScanRun {
  id: number
  company_id: string
  scanned_at: string
  jobs_fetched: number
  jobs_new: number
  error: string | null
  duration_ms: number | null
}

export interface ScanStats {
  total_scans: number
  total_fetched: number
  total_new: number
  error_count: number
  avg_duration_ms: number | null
  new_today: number
  new_this_month: number
  recent_runs: ScanRun[]
}

export interface MonthlyJobPoint {
  month: string
  new: number
  removed: number
}

export interface CompanyAnalytics {
  monthly: MonthlyJobPoint[]
  velocity: {
    this_week: number
    last_week: number
  }
}

export const ADAPTER_TYPES = [
  'greenhouse',
  'lever',
  'ashby',
  'google',
  'netflix',
  'uber',
  'apple',
  'meta',
  'microsoft',
  'rippling',
] as const

export type AdapterType = (typeof ADAPTER_TYPES)[number]

// Job status constants (mirrors model/constants.go)
export type JobStatus = 'new' | 'saved' | 'applied' | 'interviewing' | 'offer'
export type EmployType = 'full_time' | 'part_time' | 'contract' | 'internship'
export type ApplicationStage =
  | 'applied'
  | 'phone_screen'
  | 'technical'
  | 'final_round'
  | 'offer'
  | 'rejected'

export interface SalaryRange {
  min?: number
  max?: number
  currency?: string
  period?: string
}

export interface Job {
  id: string
  job_post_id: string
  company_id: string
  title: string
  location: string | null
  remote: boolean
  url: string
  department: string | null
  employ_type: string | null
  experience_years: number | null
  tech_stack: string[] | null
  salary_range: SalaryRange | null
  llm_summary: string | null
  sponsorship: boolean
  requires_citizen: boolean
  security_clearance: boolean
  role_brief: string | null
  responsibilities: string[] | null
  status: JobStatus
  saved: boolean
  notes: string | null
  added_at: string
  company: Company
}

export interface JobApplication {
  id: number
  job_result_id: string
  stage: ApplicationStage
  stage_at: string
  note: string | null
}

export interface ApplicationWithJob {
  id: number
  job_result_id: string
  stage: ApplicationStage
  stage_at: string
  note: string | null
  job_title: string
  company_id: string
  company_name: string
}

export interface QueueEntry {
  id: string
  company_id: string
  company_name: string
  adapter_type: string
  status: 'queued' | 'running'
  source: 'scheduler' | 'manual'
  priority: number
  queued_at: string
  started_at: string | null
  next_scan_at: string | null // null = never scanned (first scan / ASAP)
}

export interface DashboardStats {
  scanned_today: number
  jobs_new: number
  saved: number
  applied: number
  pipeline_pending: number
  pipeline_errors: number
}

export interface WeeklyDataPoint {
  day: string
  scanned: number
  relevant: number
}

export interface ActivityEvent {
  id: string
  type: 'job_found' | 'scan_complete' | 'job_saved' | 'job_applied' | 'job_dismissed'
  description: string
  time: string
}

// ── User Dashboard ──────────────────────────────────────────────────────────

export interface ProgressStats {
  applied_count: number
  awaiting_reply: number
  interviewing_count: number
  interviewing_this_week: number
  offer_count: number
}

export interface MarketStats {
  new_since_yesterday: number
  total_open: number
  added_this_week: number
  added_last_week: number
  weekly_delta: number
}

export interface WeeklyCount {
  week_start: string // "YYYY-MM-DD"
  job_count: number
}

export interface InterviewRow {
  application_id: number
  job_id: string
  company_name: string
  job_title: string
  stage: string
  display_stage: string
  stage_at: string
}

export interface CompanyVelocityRow {
  company_id: string
  company_name: string
  week_current: number
  week_1: number
  week_2: number
  week_3: number
  multiplier: number
  velocity_tag: 'spike' | 'rising' | 'normal'
}

export interface TechTrendRow {
  tech: string
  q1: number
  q2: number
  q3: number
  q4: number
  growth_pct: number
  adoption_pct: number
  is_new: boolean
}

export interface TechTrends {
  total_jobs: number
  rising: TechTrendRow[]
  new_entrants: TechTrendRow[]
}

export interface PipelineStats {
  total_companies: number
  active_companies: number
  queued: number
  crawling: number
  awaiting_filter: number
  awaiting_enrichment: number
  new_jobs_24h: number
  errors_24h: number
}

// ── Insights ────────────────────────────────────────────────────────────────

export interface TechCount {
  tech: string
  count: number
}

export interface CompanyWeekCount {
  company_name: string
  week: string
  count: number
}

export interface FunnelStep {
  stage: string
  count: number
}

export interface LocationCount {
  country: string
  count: number
}

export interface StateCount {
  state: string
  count: number
}

export interface LocationData {
  countries: LocationCount[]
  states: StateCount[]
}

export interface SponsorshipDay {
  date: string
  total: number
  sponsored: number
}

export interface CompanyVelocityInsight {
  company_id: string
  company_name: string
  week_current: number
  week_1: number
  week_2: number
  week_3: number
  multiplier: number
  velocity_tag: 'spike' | 'rising' | 'normal'
}

export interface RoleCategoryCount {
  category: string
  count: number
}
