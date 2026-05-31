import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { KpiCard } from '@/components/ui/kpi-card'

describe('KpiCard', () => {
  it('renders value and label as a div when not interactive', () => {
    render(<KpiCard value={42} label="Total" />)
    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText('Total')).toBeInTheDocument()
    // No button role when neither clickable nor onClick is set
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('renders a string value', () => {
    render(<KpiCard value="—" label="Empty" />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('renders as a button when onClick is provided and fires it', async () => {
    const onClick = vi.fn()
    render(<KpiCard value={1} label="Clickable" onClick={onClick} />)
    const btn = screen.getByRole('button')
    expect(btn).toHaveAttribute('type', 'button')
    await userEvent.click(btn)
    expect(onClick).toHaveBeenCalledOnce()
  })

  it('renders as a button when clickable is true even without onClick', () => {
    render(<KpiCard value={1} label="Clickable" clickable />)
    expect(screen.getByRole('button')).toBeInTheDocument()
  })

  it('applies active styling and title attribute', () => {
    render(<KpiCard value={1} label="Active" clickable active title="tooltip" />)
    const btn = screen.getByRole('button')
    expect(btn).toHaveAttribute('title', 'tooltip')
    expect(btn.className).toContain('border-red-500/40')
  })

  it('applies a custom valueClassName', () => {
    render(<KpiCard value={5} label="Styled" valueClassName="text-red-500" />)
    expect(screen.getByText('5').className).toContain('text-red-500')
  })
})
