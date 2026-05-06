import type { SalaryRange } from '@/types'

// ── Avatar color ──────────────────────────────────────────────────────────────

const AVATAR_COLORS = [
  'bg-violet-600', 'bg-blue-600', 'bg-emerald-600',
  'bg-amber-600',  'bg-rose-600', 'bg-cyan-600',
  'bg-indigo-600', 'bg-teal-600',
]

export function avatarColor(name: string): string {
  if (!name) return AVATAR_COLORS[0]
  let hash = 0
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash)
  }
  return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length]
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
  const fmt = (n: number) =>
    n >= 1000 ? `${symbol}${Math.round(n / 1000)}k` : `${symbol}${n}`
  if (salary.min && salary.max) return `${fmt(salary.min)} – ${fmt(salary.max)}`
  if (salary.min) return `${fmt(salary.min)}+`
  if (salary.max) return `up to ${fmt(salary.max)}`
  return null
}
