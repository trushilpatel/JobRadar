import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { FilterChip } from '../FilterChip'

describe('FilterChip', () => {
  it('renders the label text', () => {
    render(<FilterChip label="Remote" onRemove={() => {}} />)
    expect(screen.getByText('Remote')).toBeInTheDocument()
  })

  it('calls onRemove when the remove button is clicked', async () => {
    const onRemove = vi.fn()
    render(<FilterChip label="Remote" onRemove={onRemove} />)
    await userEvent.click(screen.getByRole('button', { name: 'Remove Remote' }))
    expect(onRemove).toHaveBeenCalledTimes(1)
  })

  it('has an accessible aria-label on the remove button', () => {
    render(<FilterChip label="Stripe" onRemove={() => {}} />)
    expect(screen.getByRole('button', { name: 'Remove Stripe' })).toBeInTheDocument()
  })
})
