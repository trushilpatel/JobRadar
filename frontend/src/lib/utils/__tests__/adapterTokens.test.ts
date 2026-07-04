import { describe, it, expect } from 'vitest'
import { adapterGradient, adapterEdge, KNOWN_ADAPTERS } from '../adapterTokens'

describe('adapterGradient', () => {
  it('returns the specific var for a known adapter', () => {
    expect(adapterGradient('greenhouse')).toBe('var(--adapter-greenhouse)')
  })

  it('returns fallback var for an unknown adapter string', () => {
    expect(adapterGradient('unknown-ats')).toBe('var(--adapter-fallback)')
  })

  it('returns fallback var for undefined', () => {
    expect(adapterGradient(undefined)).toBe('var(--adapter-fallback)')
  })

  it('returns fallback var for empty string', () => {
    expect(adapterGradient('')).toBe('var(--adapter-fallback)')
  })

  it('covers all 15 known adapters', () => {
    for (const a of KNOWN_ADAPTERS) {
      expect(adapterGradient(a)).toBe(`var(--adapter-${a})`)
    }
  })
})

describe('adapterEdge', () => {
  it('returns the specific edge var for a known adapter', () => {
    expect(adapterEdge('lever')).toBe('var(--edge-lever)')
  })

  it('returns fallback edge var for an unknown adapter', () => {
    expect(adapterEdge('nope')).toBe('var(--edge-fallback)')
  })

  it('returns fallback edge var for undefined', () => {
    expect(adapterEdge(undefined)).toBe('var(--edge-fallback)')
  })
})
