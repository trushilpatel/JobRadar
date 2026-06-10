import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Toggle, toggleVariants } from '@/components/ui/toggle'

describe('Toggle', () => {
  it('renders in the off (unpressed) state by default', () => {
    render(<Toggle aria-label="bold">B</Toggle>)
    const el = screen.getByRole('button', { name: 'bold' })
    expect(el).toHaveAttribute('data-state', 'off')
  })

  it('toggles pressed state on click', async () => {
    const onPressedChange = vi.fn()
    render(
      <Toggle aria-label="bold" onPressedChange={onPressedChange}>
        B
      </Toggle>
    )
    const el = screen.getByRole('button', { name: 'bold' })
    await userEvent.click(el)
    expect(onPressedChange).toHaveBeenCalledWith(true)
    expect(el).toHaveAttribute('data-state', 'on')
  })

  it('renders the controlled pressed state', () => {
    render(
      <Toggle aria-label="bold" pressed onPressedChange={() => {}}>
        B
      </Toggle>
    )
    expect(screen.getByRole('button', { name: 'bold' })).toHaveAttribute('data-state', 'on')
  })

  it('is disabled when disabled prop is set', () => {
    render(
      <Toggle aria-label="bold" disabled>
        B
      </Toggle>
    )
    expect(screen.getByRole('button', { name: 'bold' })).toBeDisabled()
  })

  it('applies outline variant and sm size classes', () => {
    render(
      <Toggle aria-label="bold" variant="outline" size="sm">
        B
      </Toggle>
    )
    const el = screen.getByRole('button', { name: 'bold' })
    expect(el.className).toContain('border-[var(--border)]')
    expect(el.className).toContain('h-9')
  })

  it('exposes a toggleVariants helper', () => {
    expect(toggleVariants({ variant: 'default', size: 'lg' })).toContain('h-11')
  })
})
