import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { JobLocationCell } from '../JobLocationCell'

describe('JobLocationCell', () => {
  it('renders a Remote badge with the location text when remote and a location are both set', () => {
    render(<JobLocationCell remote location="New York, NY" />)
    expect(screen.getByText('New York, NY')).toBeInTheDocument()
  })

  it('renders the literal "Remote" label when remote is true and location is null', () => {
    render(<JobLocationCell remote location={null} />)
    expect(screen.getByText('Remote')).toBeInTheDocument()
  })

  it('renders the location text with a pin icon when not remote and a location is present', () => {
    render(<JobLocationCell remote={false} location="Austin, TX" />)
    expect(screen.getByText('Austin, TX')).toBeInTheDocument()
  })

  it('renders nothing when not remote and location is null', () => {
    const { container } = render(<JobLocationCell remote={false} location={null} />)
    expect(container).toBeEmptyDOMElement()
  })
})
