import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { hiddenItems, useHiddenItems } from '@/lib/hidden-items'

const PREFIX = 'jobradar:hidden:'

describe('hiddenItems (imperative API)', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('list() returns [] when nothing stored', () => {
    expect(hiddenItems.list('companies')).toEqual([])
  })

  it('add() persists a value', () => {
    hiddenItems.add('companies', 'Stripe')
    expect(hiddenItems.list('companies')).toEqual(['Stripe'])
    expect(localStorage.getItem(PREFIX + 'companies')).toBe(JSON.stringify(['Stripe']))
  })

  it('add() is idempotent — duplicates ignored', () => {
    hiddenItems.add('companies', 'Stripe')
    hiddenItems.add('companies', 'Stripe')
    expect(hiddenItems.list('companies')).toEqual(['Stripe'])
  })

  it('has() reflects current state', () => {
    expect(hiddenItems.has('companies', 'Stripe')).toBe(false)
    hiddenItems.add('companies', 'Stripe')
    expect(hiddenItems.has('companies', 'Stripe')).toBe(true)
  })

  it('remove() drops a value', () => {
    hiddenItems.add('companies', 'Stripe')
    hiddenItems.add('companies', 'Palantir')
    hiddenItems.remove('companies', 'Stripe')
    expect(hiddenItems.list('companies')).toEqual(['Palantir'])
  })

  it('remove() is no-op for unknown values', () => {
    hiddenItems.add('companies', 'Stripe')
    hiddenItems.remove('companies', 'Unknown')
    expect(hiddenItems.list('companies')).toEqual(['Stripe'])
  })

  it('clear() empties the list', () => {
    hiddenItems.add('companies', 'A')
    hiddenItems.add('companies', 'B')
    hiddenItems.clear('companies')
    expect(hiddenItems.list('companies')).toEqual([])
  })

  it('categories are independent', () => {
    hiddenItems.add('companies', 'Stripe')
    hiddenItems.add('roles', 'Manager')
    expect(hiddenItems.list('companies')).toEqual(['Stripe'])
    expect(hiddenItems.list('roles')).toEqual(['Manager'])
  })

  it('survives corrupt localStorage payload', () => {
    localStorage.setItem(PREFIX + 'companies', 'not-json')
    expect(hiddenItems.list('companies')).toEqual([])
  })

  it('filters non-string entries from a tampered payload', () => {
    localStorage.setItem(PREFIX + 'companies', JSON.stringify(['Stripe', 42, null, 'OpenAI']))
    expect(hiddenItems.list('companies')).toEqual(['Stripe', 'OpenAI'])
  })

  it('ignores non-array JSON payloads', () => {
    localStorage.setItem(PREFIX + 'companies', JSON.stringify({ foo: 'bar' }))
    expect(hiddenItems.list('companies')).toEqual([])
  })
})

describe('useHiddenItems (React hook)', () => {
  beforeEach(() => {
    localStorage.clear()
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('returns the initial list', () => {
    hiddenItems.add('companies', 'Stripe')
    const { result } = renderHook(() => useHiddenItems('companies'))
    expect(result.current.items).toEqual(['Stripe'])
  })

  it('re-renders when add() is called', () => {
    const { result } = renderHook(() => useHiddenItems('companies'))
    expect(result.current.items).toEqual([])
    act(() => {
      result.current.add('Stripe')
    })
    expect(result.current.items).toEqual(['Stripe'])
    expect(result.current.has('Stripe')).toBe(true)
  })

  it('re-renders when remove() is called', () => {
    hiddenItems.add('companies', 'Stripe')
    const { result } = renderHook(() => useHiddenItems('companies'))
    act(() => {
      result.current.remove('Stripe')
    })
    expect(result.current.items).toEqual([])
  })

  it('re-renders when clear() is called', () => {
    hiddenItems.add('companies', 'Stripe')
    hiddenItems.add('companies', 'Palantir')
    const { result } = renderHook(() => useHiddenItems('companies'))
    act(() => {
      result.current.clear()
    })
    expect(result.current.items).toEqual([])
  })

  it('does not re-render for other categories', () => {
    const { result } = renderHook(() => useHiddenItems('companies'))
    const initial = result.current.items
    act(() => {
      hiddenItems.add('roles', 'Manager')
    })
    expect(result.current.items).toBe(initial)
  })

  it('reacts to cross-tab storage events', () => {
    const { result } = renderHook(() => useHiddenItems('companies'))
    expect(result.current.items).toEqual([])

    act(() => {
      // Simulate another tab writing to localStorage and emitting storage event
      localStorage.setItem(PREFIX + 'companies', JSON.stringify(['Stripe']))
      window.dispatchEvent(
        new StorageEvent('storage', {
          key: PREFIX + 'companies',
          newValue: JSON.stringify(['Stripe']),
        })
      )
    })

    expect(result.current.items).toEqual(['Stripe'])
  })

  it('ignores storage events for unrelated keys', () => {
    const { result } = renderHook(() => useHiddenItems('companies'))
    const initial = result.current.items
    act(() => {
      window.dispatchEvent(new StorageEvent('storage', { key: PREFIX + 'roles', newValue: 'X' }))
    })
    expect(result.current.items).toBe(initial)
  })
})
