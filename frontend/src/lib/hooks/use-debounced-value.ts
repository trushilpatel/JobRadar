import { useEffect, useState } from 'react'

// Returns a debounced copy of `value` that only updates after `delay` ms
// without further changes. Use to drive an API query off a fast-changing
// input (search box, slider) without firing a request on every keystroke.
export function useDebouncedValue<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value)

  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(id)
  }, [value, delay])

  return debounced
}
