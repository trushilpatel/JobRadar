import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ApplyQueueStatusPill } from '../ApplyQueueStatusPill'

describe('ApplyQueueStatusPill', () => {
  it('renders the Saved label', () => {
    render(<ApplyQueueStatusPill status="saved" />)
    expect(screen.getByText('Saved')).toBeInTheDocument()
  })

  it('renders the Applied label', () => {
    render(<ApplyQueueStatusPill status="applied" />)
    expect(screen.getByText('Applied')).toBeInTheDocument()
  })

  it('renders the Interviewing label', () => {
    render(<ApplyQueueStatusPill status="interviewing" />)
    expect(screen.getByText('Interviewing')).toBeInTheDocument()
  })
})
