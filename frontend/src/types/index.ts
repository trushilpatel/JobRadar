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
