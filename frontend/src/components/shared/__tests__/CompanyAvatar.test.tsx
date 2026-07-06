import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { CompanyAvatar } from '../CompanyAvatar'

describe('CompanyAvatar', () => {
  it('renders two initials for a two-word name', () => {
    render(<CompanyAvatar name="Open AI" />)
    expect(screen.getByRole('img', { name: 'Open AI' })).toHaveTextContent('OA')
  })

  it('renders two initials (first two chars) for a single-word name', () => {
    render(<CompanyAvatar name="Stripe" />)
    expect(screen.getByRole('img', { name: 'Stripe' })).toHaveTextContent('ST')
  })

  it('renders ? for empty name', () => {
    render(<CompanyAvatar name="" />)
    expect(screen.getByRole('img')).toHaveTextContent('?')
  })

  it('uses the adapter gradient background when adapterType is provided', () => {
    const { container } = render(<CompanyAvatar name="Stripe" adapterType="greenhouse" />)
    const el = container.firstChild as HTMLElement
    expect(el.style.background).toBe('var(--adapter-greenhouse)')
  })

  it('uses the fallback gradient when adapterType is not provided', () => {
    const { container } = render(<CompanyAvatar name="Stripe" />)
    const el = container.firstChild as HTMLElement
    expect(el.style.background).toBe('var(--adapter-fallback)')
  })

  it('uses the fallback gradient for an unknown adapter', () => {
    const { container } = render(<CompanyAvatar name="Stripe" adapterType="unknown-ats" />)
    const el = container.firstChild as HTMLElement
    expect(el.style.background).toBe('var(--adapter-fallback)')
  })

  it('renders sm size by default', () => {
    const { container } = render(<CompanyAvatar name="Stripe" />)
    expect(container.firstChild).toHaveClass('w-7', 'h-7')
  })

  it('renders md size when size=md', () => {
    const { container } = render(<CompanyAvatar name="Stripe" size="md" />)
    expect(container.firstChild).toHaveClass('w-9', 'h-9')
  })
})
