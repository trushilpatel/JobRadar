export const atsQueryKeys = {
  list: () => ['ats'] as const,
  detail: (name: string) => ['ats', name] as const,
}
