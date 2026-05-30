/**
 * Detects whether the current platform is macOS. Used by both `formatKeys`
 * (glyph rendering) and `buildDispatch` (`mod` expansion) so the two always
 * agree on what `mod` means.
 */
export function isMac(): boolean {
  if (typeof navigator === 'undefined') return false
  const platform = navigator.platform ?? ''
  const ua = navigator.userAgent ?? ''
  return /Mac|iPhone|iPad|iPod/i.test(platform) || /Mac/i.test(ua)
}
