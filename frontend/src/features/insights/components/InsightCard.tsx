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

export function InsightEmpty() {
  return (
    <div className="flex items-center justify-center h-32 text-muted text-sm">
      Not enough data yet
    </div>
  )
}

export function InsightError({ message = 'Failed to load data' }: { message?: string }) {
  return (
    <div className="flex items-center justify-center h-32 text-red-400/70 text-sm">{message}</div>
  )
}

export function InsightSkeleton({ height = 'h-32' }: { height?: string }) {
  return <div className={`${height} bg-subtle rounded-lg animate-pulse`} />
}
