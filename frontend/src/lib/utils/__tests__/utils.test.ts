import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { formatRelativeTime, formatExactDateTime } from '../utils'

// Fixed "now" so relative-time buckets are deterministic.
const NOW = new Date('2026-06-24T12:00:00.000Z')

// Build an ISO string for `seconds` before (or, if negative, after) NOW.
function ago(seconds: number): string {
  return new Date(NOW.getTime() - seconds * 1000).toISOString()
}

describe('formatRelativeTime', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(NOW)
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns "Never" for null / undefined / empty', () => {
    expect(formatRelativeTime(null)).toBe('Never')
    expect(formatRelativeTime(undefined)).toBe('Never')
    expect(formatRelativeTime('')).toBe('Never')
  })

  it('returns "Never" for an unparseable date (never "NaNy ago")', () => {
    expect(formatRelativeTime('not-a-date')).toBe('Never')
  })

  it('treats sub-minute and future timestamps as "Just now"', () => {
    expect(formatRelativeTime(ago(0))).toBe('Just now')
    expect(formatRelativeTime(ago(59))).toBe('Just now')
    expect(formatRelativeTime(ago(-3600))).toBe('Just now') // 1h in the future (clock skew)
  })

  it('formats minutes', () => {
    expect(formatRelativeTime(ago(60))).toBe('1m ago')
    expect(formatRelativeTime(ago(3599))).toBe('59m ago')
  })

  it('formats hours', () => {
    expect(formatRelativeTime(ago(3600))).toBe('1h ago')
    expect(formatRelativeTime(ago(86399))).toBe('23h ago')
  })

  it('formats days', () => {
    expect(formatRelativeTime(ago(86400))).toBe('1d ago')
    expect(formatRelativeTime(ago(604799))).toBe('6d ago')
  })

  it('formats weeks', () => {
    expect(formatRelativeTime(ago(604800))).toBe('1w ago')
    expect(formatRelativeTime(ago(2629799))).toBe('4w ago') // just under a month
  })

  it('formats months', () => {
    expect(formatRelativeTime(ago(2629800))).toBe('1mo ago')
    expect(formatRelativeTime(ago(31557599))).toBe('11mo ago') // just under a year
  })

  it('formats years', () => {
    expect(formatRelativeTime(ago(31557600))).toBe('1y ago')
    expect(formatRelativeTime(ago(2 * 31557600))).toBe('2y ago')
  })
})

describe('formatExactDateTime', () => {
  it('returns "Unknown" for null / undefined / empty', () => {
    expect(formatExactDateTime(null)).toBe('Unknown')
    expect(formatExactDateTime(undefined)).toBe('Unknown')
    expect(formatExactDateTime('')).toBe('Unknown')
  })

  it('returns "Unknown" for an unparseable date', () => {
    expect(formatExactDateTime('not-a-date')).toBe('Unknown')
  })

  it('renders a locale timestamp that carries the year and time', () => {
    const out = formatExactDateTime('2026-06-12T14:14:00.000Z')
    expect(out).not.toBe('Unknown')
    expect(out).toContain('2026')
    // Includes a minute component regardless of locale/timezone.
    expect(out).toMatch(/\d:\d{2}/)
  })
})
