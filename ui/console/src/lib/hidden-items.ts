import { useEffect, useState, useCallback } from 'react'

/**
 * localStorage-backed registry for "items the user has chosen to hide forever"
 * (companies they've blocked, role types they don't want, etc.).
 *
 * Generic by category — same primitive works for any hideable thing.
 * Cross-tab safe via the `storage` event + a custom event for same-tab updates.
 */

const PREFIX = 'jobradar:hidden:'
const CHANGE_EVENT = 'jobradar:hidden-changed'

interface ChangeDetail {
  category: string
}

const readList = (category: string): string[] => {
  try {
    const raw = localStorage.getItem(PREFIX + category)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter((v): v is string => typeof v === 'string')
  } catch {
    return []
  }
}

const writeList = (category: string, list: string[]): void => {
  localStorage.setItem(PREFIX + category, JSON.stringify(list))
  window.dispatchEvent(new CustomEvent<ChangeDetail>(CHANGE_EVENT, { detail: { category } }))
}

/**
 * Imperative API — call from event handlers, mutation success callbacks, etc.
 * Component re-renders should use the {@link useHiddenItems} hook instead.
 */
export const hiddenItems = {
  list(category: string): string[] {
    return readList(category)
  },
  has(category: string, value: string): boolean {
    return readList(category).includes(value)
  },
  add(category: string, value: string): void {
    const current = readList(category)
    if (current.includes(value)) return
    writeList(category, [...current, value])
  },
  remove(category: string, value: string): void {
    const current = readList(category)
    if (!current.includes(value)) return
    writeList(
      category,
      current.filter((v) => v !== value)
    )
  },
  clear(category: string): void {
    writeList(category, [])
  },
}

export interface UseHiddenItemsResult {
  items: string[]
  has: (value: string) => boolean
  add: (value: string) => void
  remove: (value: string) => void
  clear: () => void
}

/**
 * React hook that subscribes to a category's hidden list.
 *
 * Re-renders when:
 *  - same-tab: any code calls hiddenItems.add/remove/clear for this category
 *  - cross-tab: another tab modifies the same localStorage key
 */
export function useHiddenItems(category: string): UseHiddenItemsResult {
  const [items, setItems] = useState<string[]>(() => readList(category))

  useEffect(() => {
    const refresh = () => setItems(readList(category))

    const onCustom = (e: Event) => {
      const detail = (e as CustomEvent<ChangeDetail>).detail
      if (detail.category === category) refresh()
    }

    const onStorage = (e: StorageEvent) => {
      if (e.key === PREFIX + category) refresh()
    }

    window.addEventListener(CHANGE_EVENT, onCustom)
    window.addEventListener('storage', onStorage)
    refresh()
    return () => {
      window.removeEventListener(CHANGE_EVENT, onCustom)
      window.removeEventListener('storage', onStorage)
    }
  }, [category])

  const has = useCallback((value: string) => items.includes(value), [items])
  const add = useCallback((value: string) => hiddenItems.add(category, value), [category])
  const remove = useCallback((value: string) => hiddenItems.remove(category, value), [category])
  const clear = useCallback(() => hiddenItems.clear(category), [category])

  return { items, has, add, remove, clear }
}
