import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { StatusPill, type StatusKind } from '@/components/ui/status-pill'

const cases: { status: StatusKind; text: string }[] = [
  { status: 'ok', text: 'OK' },
  { status: 'error', text: 'Error' },
  { status: 'running', text: 'Running' },
  { status: 'queued', text: 'Queued' },
  { status: 'pending', text: 'Never' },
]

describe('StatusPill', () => {
  it.each(cases)('renders the default text for the $status variant', ({ status, text }) => {
    render(<StatusPill status={status} />)
    expect(screen.getByText(text)).toBeInTheDocument()
  })

  it('renders a custom label overriding the default text', () => {
    render(<StatusPill status="ok" label="Healthy" />)
    expect(screen.getByText('Healthy')).toBeInTheDocument()
    expect(screen.queryByText('OK')).not.toBeInTheDocument()
  })

  it('adds the pulse animation only for the running variant', () => {
    const { container } = render(<StatusPill status="running" />)
    const dot = container.querySelector('span > span')
    expect(dot?.className).toContain('animate-pulse')
  })

  it('does not add the pulse animation for non-running variants', () => {
    const { container } = render(<StatusPill status="ok" />)
    const dot = container.querySelector('span > span')
    expect(dot?.className).not.toContain('animate-pulse')
  })

  it('merges a custom className', () => {
    const { container } = render(<StatusPill status="ok" className="custom-x" />)
    expect((container.firstChild as HTMLElement).className).toContain('custom-x')
  })
})
