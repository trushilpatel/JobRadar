import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { z } from 'zod'
import { parseItemsResilient, setSchemaDriftLogger } from '../resilient-parse'

const Item = z.object({ id: z.string(), n: z.number() })

describe('parseItemsResilient', () => {
  const drift = vi.fn()
  let restore: ReturnType<typeof setSchemaDriftLogger>

  beforeEach(() => {
    drift.mockClear()
    restore = setSchemaDriftLogger(drift)
  })
  afterEach(() => {
    setSchemaDriftLogger(restore)
  })

  it('keeps valid items and drops invalid ones', () => {
    const out = parseItemsResilient(
      Item,
      [{ id: 'a', n: 1 }, { id: 'b' }, { id: 'c', n: 3 }],
      'test'
    )
    expect(out).toEqual([
      { id: 'a', n: 1 },
      { id: 'c', n: 3 },
    ])
  })

  it('logs each dropped item with context and index', () => {
    parseItemsResilient(Item, [{ id: 'a', n: 1 }, { bad: true }], 'jobs')
    expect(drift).toHaveBeenCalledTimes(1)
    expect(drift).toHaveBeenCalledWith('jobs', 1, expect.stringContaining('n'))
  })

  it('never throws on fully malformed input', () => {
    expect(() => parseItemsResilient(Item, [null, 42, 'x', undefined], 'test')).not.toThrow()
    const out = parseItemsResilient(Item, [null, 42], 'test')
    expect(out).toEqual([])
  })

  it('returns an empty array for an empty input', () => {
    expect(parseItemsResilient(Item, [], 'test')).toEqual([])
    expect(drift).not.toHaveBeenCalled()
  })
})
