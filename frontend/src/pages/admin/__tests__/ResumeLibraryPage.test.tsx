import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { ResumeLibraryPage } from '../ResumeLibraryPage'

// Mocked via an inline factory (not a deep `import`) so the architecture
// lint rule (pages/ may only import feature barrels) isn't tripped — see
// AdminUsersPage.seam.test.tsx for the barrel-reexport alternative used
// elsewhere; the resumes barrel deliberately doesn't re-export the raw api
// functions since pages should only consume them via hooks. `vi.hoisted`
// is required because `vi.mock` factories are hoisted above module scope.
const { fetchResumesMock } = vi.hoisted(() => ({ fetchResumesMock: vi.fn() }))
vi.mock('@/features/resumes/api/resumes.api', () => ({
  fetchResumes: fetchResumesMock,
}))

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0, gcTime: 0 } } })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <MemoryRouter>
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      </MemoryRouter>
    )
  }
}

const sampleResume = {
  id: 'r1',
  user_id: 'u1',
  label: 'My resume',
  original_filename: 'resume.pdf',
  content_type: 'application/pdf' as const,
  size_bytes: 2048,
  created_at: '2026-07-03T00:00:00Z',
  keywords: [] as string[],
}

describe('ResumeLibraryPage', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows the empty state and opens the upload dialog on click', async () => {
    fetchResumesMock.mockResolvedValue([])
    render(<ResumeLibraryPage />, { wrapper: makeWrapper() })

    await waitFor(() => expect(screen.getByText('No resumes yet')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: /Upload first resume/i }))
    expect(screen.getByLabelText(/File/i)).toBeInTheDocument()
  })

  it('renders a card for each resume once the list resolves', async () => {
    fetchResumesMock.mockResolvedValue([sampleResume])
    render(<ResumeLibraryPage />, { wrapper: makeWrapper() })

    await waitFor(() => expect(screen.getByText('My resume')).toBeInTheDocument())
    expect(screen.getByText('resume.pdf')).toBeInTheDocument()
  })

  it('shows an error state with a retry button when the fetch fails', async () => {
    fetchResumesMock.mockRejectedValue(new Error('network error'))
    render(<ResumeLibraryPage />, { wrapper: makeWrapper() })

    await waitFor(() =>
      expect(screen.getByText('Failed to load. Please try again.')).toBeInTheDocument()
    )
    expect(screen.getByText('Retry')).toBeInTheDocument()
  })

  it('opens the upload dialog when "Upload resume" is clicked', async () => {
    fetchResumesMock.mockResolvedValue([sampleResume])
    render(<ResumeLibraryPage />, { wrapper: makeWrapper() })

    await waitFor(() => expect(screen.getByText('My resume')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: /Upload resume/i }))
    expect(screen.getByLabelText(/File/i)).toBeInTheDocument()
  })

  it('disables the upload button and shows the limit banner at the resume cap', async () => {
    fetchResumesMock.mockResolvedValue(
      Array.from({ length: 50 }, (_, i) => ({ ...sampleResume, id: `r${i}` }))
    )
    render(<ResumeLibraryPage />, { wrapper: makeWrapper() })

    await waitFor(() => expect(screen.getByText(/Resume limit reached/)).toBeInTheDocument())
    expect(screen.getByRole('button', { name: /Upload resume/i })).toBeDisabled()
  })
})
