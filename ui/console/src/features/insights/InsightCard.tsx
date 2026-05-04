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
    <div className="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
      <div className="mb-4">
        <h2 className="text-sm font-semibold text-white">{title}</h2>
        <p className="text-xs text-slate-500 mt-0.5">{subtitle}</p>
      </div>
      {children}
    </div>
  )
}

export function InsightEmpty() {
  return (
    <div className="flex items-center justify-center h-32 text-slate-500 text-sm">
      Not enough data yet
    </div>
  )
}

export function InsightSkeleton({ height = 'h-32' }: { height?: string }) {
  return <div className={`${height} bg-white/[0.03] rounded-lg animate-pulse`} />
}

