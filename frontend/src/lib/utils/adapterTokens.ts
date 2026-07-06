export const KNOWN_ADAPTERS = [
  'greenhouse',
  'lever',
  'ashby',
  'workday',
  'google',
  'smartrecruiters',
  'apple',
  'meta',
  'microsoft',
  'netflix',
  'uber',
  'deel',
  'gem',
  'rippling',
  'eightfold',
] as const

const KNOWN_SET = new Set<string>(KNOWN_ADAPTERS)

export function adapterGradient(adapterType: string | undefined): string {
  if (adapterType && KNOWN_SET.has(adapterType)) {
    return `var(--adapter-${adapterType})`
  }
  return 'var(--adapter-fallback)'
}

export function adapterEdge(adapterType: string | undefined): string {
  if (adapterType && KNOWN_SET.has(adapterType)) {
    return `var(--edge-${adapterType})`
  }
  return 'var(--edge-fallback)'
}
