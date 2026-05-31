/**
 * Centralised query-key factory for the auth/admin feature.
 * Always import from here — never inline ['admin-users', ...] or
 * ['admin-audit', ...] arrays in hooks or components.
 */
export const authQueryKeys = {
  adminUsersBase: () => ['admin-users'] as const,
  adminUsers: (filter: object, page: number, limit: number) =>
    ['admin-users', filter, page, limit] as const,
  adminAuditBase: () => ['admin-audit'] as const,
  adminAudit: (filter: object, page: number, limit: number) =>
    ['admin-audit', filter, page, limit] as const,
}
