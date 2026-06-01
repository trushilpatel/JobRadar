import type { AdminCompany, ScanSettingsView } from '../api/adminCompanies.api'
import type { Company } from '@/types'

function deriveLastScanStatus(
  s: ScanSettingsView | null,
  queueStatus: string | undefined
): 'ok' | 'error' | 'pending' | 'running' | 'queued' | null {
  if (queueStatus === 'running') return 'running'
  if (queueStatus === 'queued') return 'queued'
  if (!s) return null
  if (s.consecutive_errors > 0) return 'error'
  if (s.last_scan_at) return 'ok'
  return null
}

export function normalizeAdminCompany(c: AdminCompany): Company {
  const s = c.scan_settings
  return {
    id: c.id,
    name: c.name,
    url: c.website ?? '',
    adapter_type: s?.adapter_type ?? '',
    active: s?.active ?? false,
    created_at: c.created_at,
    last_scan_at: s?.last_scan_at ?? null,
    last_scan_status: deriveLastScanStatus(s, c.queue_status),
    scan_history: c.scan_history,
    scan_settings: s
      ? {
          scan_interval_override: s.scan_interval_override ?? null,
          consecutive_errors: s.consecutive_errors,
          last_error_message: s.last_error_message ?? null,
          last_error_at: s.last_error_at ?? null,
        }
      : undefined,
  }
}
