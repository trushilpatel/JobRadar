export const QUEUE_STATUS = {
  QUEUED: 'queued',
  RUNNING: 'running',
} as const

export type QueueStatus = (typeof QUEUE_STATUS)[keyof typeof QUEUE_STATUS]

export const SCAN_STATUS = {
  OK: 'ok',
  ERROR: 'error',
  PENDING: 'pending',
} as const

export type ScanStatus = (typeof SCAN_STATUS)[keyof typeof SCAN_STATUS]

export const SCAN_STATUS_FILTER = {
  ALL: 'all',
  OK: 'ok',
  ERROR: 'error',
  RUNNING: 'running',
  QUEUED: 'queued',
} as const

export type ScanStatusFilter = (typeof SCAN_STATUS_FILTER)[keyof typeof SCAN_STATUS_FILTER]

export const ACTIVE_FILTER = {
  ALL: 'all',
  ACTIVE: 'active',
  INACTIVE: 'inactive',
} as const

export type ActiveFilter = (typeof ACTIVE_FILTER)[keyof typeof ACTIVE_FILTER]
