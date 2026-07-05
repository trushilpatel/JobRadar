import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ResumeStorageMeter } from '../ResumeStorageMeter'

describe('ResumeStorageMeter', () => {
  it('renders the count against the limit', () => {
    render(<ResumeStorageMeter count={5} />)
    expect(screen.getByText('5')).toBeInTheDocument()
    expect(screen.getByText('/ 50')).toBeInTheDocument()
  })

  it('shows the normal (violet) fill below the near-limit threshold', () => {
    const { container } = render(<ResumeStorageMeter count={5} />)
    expect(container.querySelector('.bg-violet-500')).toBeInTheDocument()
  })

  it('shows the near-limit (amber) fill at 80% or more', () => {
    const { container } = render(<ResumeStorageMeter count={40} />)
    expect(container.querySelector('.bg-amber-400')).toBeInTheDocument()
  })

  it('shows the full (red) fill and red count text at the limit', () => {
    const { container } = render(<ResumeStorageMeter count={50} />)
    expect(container.querySelector('.bg-red-500')).toBeInTheDocument()
    expect(screen.getByText('50')).toHaveClass('text-red-300')
  })
})
