import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LogoMark, LogoSweepingMark, LogoLockup } from '@/components/brand/Logo'

describe('LogoMark', () => {
  it('renders with the default size and accessible label', () => {
    render(<LogoMark />)
    const img = screen.getByRole('img', { name: 'JobRadar' })
    expect(img.getAttribute('class')).toContain('h-6')
  })

  it('merges a custom className', () => {
    render(<LogoMark className="text-violet-500" />)
    expect(screen.getByRole('img', { name: 'JobRadar' }).getAttribute('class')).toContain(
      'text-violet-500'
    )
  })
})

describe('LogoSweepingMark', () => {
  it('renders the scanning label and the animated sweep group', () => {
    const { container } = render(<LogoSweepingMark />)
    expect(screen.getByRole('img', { name: 'JobRadar scanning' })).toBeInTheDocument()
    expect(container.querySelector('.motion-safe\\:animate-spin')).toBeInTheDocument()
  })
})

describe('LogoLockup', () => {
  it('renders mark + wordmark at the default (md) size', () => {
    const { container } = render(<LogoLockup />)
    expect(screen.getByText('Job')).toBeInTheDocument()
    expect(screen.getByText('Radar')).toBeInTheDocument()
    expect(container.querySelector('.text-base')).toBeInTheDocument()
  })

  it('renders the sm size', () => {
    const { container } = render(<LogoLockup size="sm" />)
    expect(container.querySelector('.text-sm')).toBeInTheDocument()
    expect(container.querySelector('.h-4')).toBeInTheDocument()
  })

  it('renders the lg size', () => {
    const { container } = render(<LogoLockup size="lg" />)
    expect(container.querySelector('.text-2xl')).toBeInTheDocument()
    expect(container.querySelector('.h-7')).toBeInTheDocument()
  })

  it('merges a custom className on the wrapper', () => {
    const { container } = render(<LogoLockup className="custom-x" />)
    expect((container.firstChild as HTMLElement).className).toContain('custom-x')
  })
})
