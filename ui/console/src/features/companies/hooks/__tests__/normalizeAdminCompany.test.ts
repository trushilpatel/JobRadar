import { describe, it, expect } from 'vitest'
import { normalizeAdminCompany } from '../normalizeAdminCompany'
import type { AdminCompany, ScanSettingsView } from '@/api/adminCompanies'

const BASE_SETTINGS: ScanSettingsView = {
  company_id: 'stripe',
  adapter_type: 'greenhouse',
  adapter_config: null,
  scan_interval_override: null,
  active: true,
  last_scan_at: null,
  last_success_at: null,
  last_error_at: null,
  last_error_message: null,
  consecutive_errors: 0,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

const BASE_COMPANY: AdminCompany = {
  id: 'stripe',
  name: 'Stripe',
  website: 'https://stripe.com/jobs',
  created_at: '2024-01-01T00:00:00Z',
  scan_settings: BASE_SETTINGS,
}

describe('normalizeAdminCompany', () => {
  it('maps flat fields from scan_settings', () => {
    const result = normalizeAdminCompany(BASE_COMPANY)
    expect(result.id).toBe('stripe')
    expect(result.name).toBe('Stripe')
    expect(result.url).toBe('https://stripe.com/jobs')
    expect(result.adapter_type).toBe('greenhouse')
    expect(result.active).toBe(true)
  })

  it('derives last_scan_status as ok when last_scan_at is set and no errors', () => {
    const company: AdminCompany = {
      ...BASE_COMPANY,
      scan_settings: { ...BASE_SETTINGS, last_scan_at: '2024-06-01T12:00:00Z', consecutive_errors: 0 },
    }
    expect(normalizeAdminCompany(company).last_scan_status).toBe('ok')
  })

  it('derives last_scan_status as error when consecutive_errors > 0', () => {
    const company: AdminCompany = {
      ...BASE_COMPANY,
      scan_settings: { ...BASE_SETTINGS, last_scan_at: '2024-06-01T12:00:00Z', consecutive_errors: 2 },
    }
    expect(normalizeAdminCompany(company).last_scan_status).toBe('error')
  })

  it('returns null last_scan_status when scan has never run', () => {
    const company: AdminCompany = {
      ...BASE_COMPANY,
      scan_settings: { ...BASE_SETTINGS, last_scan_at: null, consecutive_errors: 0 },
    }
    expect(normalizeAdminCompany(company).last_scan_status).toBeNull()
  })

  it('returns empty defaults when scan_settings is null', () => {
    const company: AdminCompany = { ...BASE_COMPANY, scan_settings: null }
    const result = normalizeAdminCompany(company)
    expect(result.adapter_type).toBe('')
    expect(result.active).toBe(false)
    expect(result.last_scan_at).toBeNull()
    expect(result.last_scan_status).toBeNull()
  })

  it('leaves scan_history undefined', () => {
    expect(normalizeAdminCompany(BASE_COMPANY).scan_history).toBeUndefined()
  })
})
