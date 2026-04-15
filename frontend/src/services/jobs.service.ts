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

export function formatSalary(salary: SalaryRange | null | undefined): string | null {
  if (!salary) return null
  const currency = salary.currency ?? '$'
  const fmt = (n: number) =>
    n >= 1000 ? `${currency}${Math.round(n / 1000)}k` : `${currency}${n}`
  if (salary.min && salary.max) return `${fmt(salary.min)} – ${fmt(salary.max)}`
  if (salary.min) return `${fmt(salary.min)}+`
  if (salary.max) return `up to ${fmt(salary.max)}`
  return null
}
