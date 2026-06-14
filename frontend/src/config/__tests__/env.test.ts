import { describe, it, expect } from 'vitest'

describe('ENV', () => {
  it('exports CLERK_PUBLISHABLE_KEY when env var is set', async () => {
    const { ENV } = await import('../env')
    expect(ENV).toHaveProperty('CLERK_PUBLISHABLE_KEY')
    expect(typeof ENV.CLERK_PUBLISHABLE_KEY).toBe('string')
  })
})
