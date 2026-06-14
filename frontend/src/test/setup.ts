import '@testing-library/jest-dom'
import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'

// Unmount any mounted components after every test across the whole suite. This
// is the canonical Testing Library pattern; it also lets react-hotkeys-hook
// (which attaches document-level keydown listeners on mount) detach those
// listeners between tests so a stale keyboard engine from one test file cannot
// leak into another and corrupt cross-file key assertions.
afterEach(() => {
  cleanup()
  // react-hotkeys-hook keeps a module-level set of currently-pressed keys that
  // it only clears on keyup or window blur. A test that fires keydown without a
  // matching keyup would otherwise leak a "held" key into the next test (even
  // in another file in the same worker) and corrupt modifier/chord resolution.
  // A window blur flushes that tracker between every test.
  window.dispatchEvent(new Event('blur'))
})

class MockResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver
}

if (typeof Element !== 'undefined' && !Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = function () {}
}

if (typeof window !== 'undefined' && !window.matchMedia) {
  window.matchMedia = (query: string) =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }) as MediaQueryList
}
