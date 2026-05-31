import { useCallback } from 'react'

interface ResizeHandleProps {
  width: number
  onChange: (next: number) => void
  min?: number
  max?: number
}

export function ResizeHandle({ width, onChange, min = 280, max = 680 }: ResizeHandleProps) {
  const startDrag = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault()
      const startX = e.clientX
      const startWidth = width
      const onMove = (ev: MouseEvent) =>
        onChange(Math.min(Math.max(startWidth + (startX - ev.clientX), min), max))
      const onUp = () => {
        document.removeEventListener('mousemove', onMove)
        document.removeEventListener('mouseup', onUp)
      }
      document.addEventListener('mousemove', onMove)
      document.addEventListener('mouseup', onUp)
    },
    [width, min, max, onChange]
  )

  return (
    <div
      onMouseDown={startDrag}
      role="separator"
      aria-orientation="vertical"
      className="w-1 shrink-0 cursor-col-resize bg-white/[0.06] hover:bg-violet-500/50 transition-colors"
    />
  )
}
