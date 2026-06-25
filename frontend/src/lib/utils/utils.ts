import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
import type { SalaryRange } from '@/types'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// Relative-time bucket thresholds, in seconds. Month/year use average lengths
// (365.25 days, and a year / 12) so the buckets tile cleanly without calendar
// math — close enough for an "added 3w ago" affordance.
const SECONDS_PER = {
  minute: 60,
  hour: 60 * 60,
  day: 60 * 60 * 24,
  week: 60 * 60 * 24 * 7,
  month: Math.floor((60 * 60 * 24 * 365.25) / 12),
  year: Math.floor(60 * 60 * 24 * 365.25),
} as const

/**
 * Terse, human relative time: "Just now", "5m ago", "3h ago", "2d ago",
 * "3w ago", "4mo ago", "1y ago". Scales to any age so old jobs never show
 * "412d ago". Pair with formatExactDateTime for an on-hover exact timestamp.
 */
export function formatRelativeTime(dateStr: string | null | undefined): string {
  if (!dateStr) return 'Never'
  const ms = new Date(dateStr).getTime()
  if (Number.isNaN(ms)) return 'Never'
  const seconds = Math.floor((Date.now() - ms) / 1000)
  if (seconds < SECONDS_PER.minute) return 'Just now'
  if (seconds < SECONDS_PER.hour) return `${Math.floor(seconds / SECONDS_PER.minute)}m ago`
  if (seconds < SECONDS_PER.day) return `${Math.floor(seconds / SECONDS_PER.hour)}h ago`
  if (seconds < SECONDS_PER.week) return `${Math.floor(seconds / SECONDS_PER.day)}d ago`
  if (seconds < SECONDS_PER.month) return `${Math.floor(seconds / SECONDS_PER.week)}w ago`
  if (seconds < SECONDS_PER.year) return `${Math.floor(seconds / SECONDS_PER.month)}mo ago`
  return `${Math.floor(seconds / SECONDS_PER.year)}y ago`
}

/**
 * Absolute, locale-aware timestamp for tooltips — e.g. "Jun 12, 2026, 2:14 PM".
 * Lets a terse "3w ago" cell reveal the exact date on hover.
 */
export function formatExactDateTime(dateStr: string | null | undefined): string {
  if (!dateStr) return 'Unknown'
  const date = new Date(dateStr)
  if (Number.isNaN(date.getTime())) return 'Unknown'
  return date.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}

// ── Avatar color ──────────────────────────────────────────────────────────────

const AVATAR_COLORS = [
  'bg-violet-600',
  'bg-blue-600',
  'bg-emerald-600',
  'bg-amber-600',
  'bg-rose-600',
  'bg-cyan-600',
  'bg-indigo-600',
  'bg-teal-600',
] as const

const FALLBACK_COLOR = AVATAR_COLORS[0]

export function avatarColor(name: string): string {
  if (!name) return FALLBACK_COLOR
  let hash = 0
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash)
  }
  return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length] ?? FALLBACK_COLOR
}

// ── Salary formatting ─────────────────────────────────────────────────────────

// Maps ISO 4217 currency codes → display symbols. Unknown codes fall back to
// the raw code so we never silently swallow the currency information.
const CURRENCY_SYMBOLS: Record<string, string> = {
  USD: '$',
  CAD: 'CA$',
  GBP: '£',
  EUR: '€',
  AUD: 'A$',
  NZD: 'NZ$',
  INR: '₹',
  JPY: '¥',
  CNY: '¥',
  SGD: 'S$',
  CHF: 'CHF',
}

export function formatSalary(salary: SalaryRange | null | undefined): string | null {
  if (!salary) return null
  // Resolve ISO code → symbol so we never render "USD137k" or "$ USD126k".
  const rawCurrency = (salary.currency ?? '').toUpperCase()
  const symbol = CURRENCY_SYMBOLS[rawCurrency] ?? (salary.currency || '$')
  const fmt = (n: number) => (n >= 1000 ? `${symbol}${Math.round(n / 1000)}k` : `${symbol}${n}`)
  if (salary.min && salary.max) return `${fmt(salary.min)} – ${fmt(salary.max)}`
  if (salary.min) return `${fmt(salary.min)}+`
  if (salary.max) return `up to ${fmt(salary.max)}`
  return null
}
