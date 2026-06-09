export const settingsQueryKeys = {
  companyScanConfig: (id: string) => ['company-scan-config', id] as const,
}

/**
 * Cross-feature cache roots that useScanIntervalOverride must invalidate.
 * Declared here (not imported from companies/) to respect feature boundaries —
 * features cannot import each other. If the companies feature renames its root,
 * update this constant and the test that guards the contract will catch any
 * remaining drift.
 */
export const scanIntervalOverrideMutationInvalidationRoots = {
  /** Mirrors companyQueryKeys.adminCompaniesBase() from features/companies */
  adminCompanies: ['admin-companies'] as const,
} as const
