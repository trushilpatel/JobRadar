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
  return {
    id: c.id,
    name: c.name,
    url: c.website,
    adapter_type: c.scan_settings?.adapter_type ?? '',
    active: c.scan_settings?.active ?? false,
    created_at: c.created_at,
    last_scan_at: c.scan_settings?.last_scan_at ?? null,
    last_scan_status: deriveLastScanStatus(c.scan_settings, c.queue_status),
    scan_history: c.scan_history,
  }
}
