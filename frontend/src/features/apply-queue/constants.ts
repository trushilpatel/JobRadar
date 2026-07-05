// Apply Queue filter values — the only two views the SegmentedSwitch offers.
// Defined once here so the hook, the page, and any future consumer never
// sprinkle raw 'all' / 'saved' strings.
export const APPLY_QUEUE_FILTER = {
  ALL: 'all',
  SAVED: 'saved',
} as const

export type ApplyQueueFilter = (typeof APPLY_QUEUE_FILTER)[keyof typeof APPLY_QUEUE_FILTER]

export const APPLY_QUEUE_LIMIT_OPTIONS = [20, 50, 100] as const
export const DEFAULT_APPLY_QUEUE_LIMIT = 20
