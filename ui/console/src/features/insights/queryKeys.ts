export const insightQueryKeys = {
  techStack: (limit: number) => ['insights', 'tech-stack', limit] as const,
  companyActivity: () => ['insights', 'company-activity'] as const,
  funnel: () => ['insights', 'funnel'] as const,
  location: () => ['insights', 'location'] as const,
  sponsorshipTrend: (days: number) => ['insights', 'sponsorship-trend', days] as const,
  velocity: (limit: number) => ['insights', 'velocity', limit] as const,
  roleCategories: () => ['insights', 'role-categories'] as const,
}
