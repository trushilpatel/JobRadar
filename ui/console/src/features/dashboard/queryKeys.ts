export const dashboardQueryKeys = {
  stats: () => ['dashboard', 'stats'] as const,
  activity: (days: number) => ['dashboard', 'activity', days] as const,
  progress: () => ['dashboard', 'progress'] as const,
  market: () => ['dashboard', 'market'] as const,
  heatmap: () => ['dashboard', 'heatmap'] as const,
  velocity: () => ['dashboard', 'velocity'] as const,
  techTrends: () => ['dashboard', 'tech-trends'] as const,
  interviews: () => ['dashboard', 'interviews'] as const,
}
