import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Switch } from '@/components/ui/switch'

describe('Switch', () => {
  it('renders unchecked by default', () => {
    render(<Switch id="test-switch" />)
    const el = screen.getByRole('switch')
    expect(el).toBeInTheDocument()
    expect(el).toHaveAttribute('data-state', 'unchecked')
  })

  it('renders checked when checked prop is true', () => {
    render(<Switch id="test-switch" checked onCheckedChange={() => {}} />)
    const el = screen.getByRole('switch')
    expect(el).toHaveAttribute('data-state', 'checked')
  })

  it('is disabled when disabled prop is true', () => {
    render(<Switch id="test-switch" disabled />)
    const el = screen.getByRole('switch')
    expect(el).toBeDisabled()
  })
})
