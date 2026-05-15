export const settingsQueryKeys = {
  adapterConfigs: () => ['adapter-configs'] as const,
  companyScanConfig: (id: string) => ['scan-config', id] as const,
}
