export interface Company {
  id: string
  name: string
  url: string
  adapter_type: string
  active: boolean
  created_at: string
  last_scan_at: string | null
  last_scan_status: 'ok' | 'error' | 'pending' | null
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
  recent_runs: ScanRun[]
}

export const ADAPTER_TYPES = [
  'greenhouse',
  'lever',
  'ashby',
  'workday',
  'workable',
  'chromedp',
] as const

export type AdapterType = typeof ADAPTER_TYPES[number]

// Job status constants (mirrors model/constants.go)
export type JobStatus = 'new' | 'saved' | 'applied'
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
  responsibilities: string | null
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

// TODO: replace with GET /api/stats when backend endpoint exists
export interface DashboardStats {
  scanned_today: number
  relevant_today: number
  saved: number
  applied: number
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
