import { AlertCircle } from 'lucide-react'

export function InsightCard({
  title,
  subtitle,
  children,
}: {
  title: string
  subtitle: string
  children: React.ReactNode
}) {
  return (
    <div className="bg-subtle border border-[var(--border)] rounded-xl p-5">
      <div className="mb-4">
        <h2 className="text-sm font-semibold text-primary">{title}</h2>
        <p className="text-xs text-muted mt-0.5">{subtitle}</p>
      </div>
      {children}
    </div>
  )
}

export function InsightEmpty({ reason }: { reason?: string }) {
  return (
    <div className="flex items-center justify-center h-32 text-placeholder text-sm">
      {reason ?? 'Not enough data yet'}
    </div>
  )
}

export function InsightError({ message = 'Failed to load data' }: { message?: string }) {
  return (
    <div className="flex items-center justify-center gap-2 h-32 rounded-lg bg-red-500/5 border border-red-500/10">
      <AlertCircle size={14} className="text-red-400 shrink-0" />
      <span className="text-red-400 text-sm">{message}</span>
    </div>
  )
}

export function InsightSkeleton({ height = 'h-32' }: { height?: string }) {
  return <div className={`${height} bg-subtle rounded-lg animate-pulse`} />
}
