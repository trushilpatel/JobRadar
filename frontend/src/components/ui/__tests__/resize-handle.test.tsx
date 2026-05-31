import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ResizeHandle } from '@/components/ui/resize-handle'

describe('ResizeHandle', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders a vertical separator', () => {
    render(<ResizeHandle width={400} onChange={() => {}} />)
    const handle = screen.getByRole('separator')
    expect(handle).toHaveAttribute('aria-orientation', 'vertical')
  })

  it('drags to a new width within bounds (mouse moving left increases width)', () => {
    const onChange = vi.fn()
    render(<ResizeHandle width={400} onChange={onChange} />)
    const handle = screen.getByRole('separator')

    fireEvent.mouseDown(handle, { clientX: 500 })
    // Move left by 50px → startWidth + (500 - 450) = 450
    fireEvent.mouseMove(document, { clientX: 450 })
    expect(onChange).toHaveBeenLastCalledWith(450)

    fireEvent.mouseUp(document)
  })

  it('clamps the new width to the max bound', () => {
    const onChange = vi.fn()
    render(<ResizeHandle width={400} onChange={onChange} min={280} max={680} />)
    const handle = screen.getByRole('separator')

    fireEvent.mouseDown(handle, { clientX: 500 })
    // Move far left → would compute 1400, clamped to max 680
    fireEvent.mouseMove(document, { clientX: -500 })
    expect(onChange).toHaveBeenLastCalledWith(680)
    fireEvent.mouseUp(document)
  })

  it('clamps the new width to the min bound', () => {
    const onChange = vi.fn()
    render(<ResizeHandle width={400} onChange={onChange} min={280} max={680} />)
    const handle = screen.getByRole('separator')

    fireEvent.mouseDown(handle, { clientX: 500 })
    // Move far right → would compute negative, clamped to min 280
    fireEvent.mouseMove(document, { clientX: 2000 })
    expect(onChange).toHaveBeenLastCalledWith(280)
    fireEvent.mouseUp(document)
  })

  it('detaches listeners on mouseup so no further changes fire', () => {
    const onChange = vi.fn()
    render(<ResizeHandle width={400} onChange={onChange} />)
    const handle = screen.getByRole('separator')

    fireEvent.mouseDown(handle, { clientX: 500 })
    fireEvent.mouseUp(document)
    onChange.mockClear()
    fireEvent.mouseMove(document, { clientX: 100 })
    expect(onChange).not.toHaveBeenCalled()
  })
})
