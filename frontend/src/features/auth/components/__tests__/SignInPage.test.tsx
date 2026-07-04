import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

function makeSignIn(overrides: Record<string, unknown> = {}) {
  return {
    status: 'needs_first_factor',
    emailCode: {
      sendCode: vi.fn().mockResolvedValue({ error: null }),
      verifyCode: vi.fn().mockResolvedValue({ error: null }),
    },
    sso: vi.fn(),
    finalize: vi.fn().mockResolvedValue({ error: null }),
    ...overrides,
  }
}

function makeSignUp(overrides: Record<string, unknown> = {}) {
  return {
    status: 'missing_requirements',
    create: vi.fn().mockResolvedValue({ error: null }),
    verifications: {
      sendEmailCode: vi.fn().mockResolvedValue({ error: null }),
      verifyEmailCode: vi.fn().mockResolvedValue({ error: null }),
    },
    sso: vi.fn(),
    finalize: vi.fn().mockResolvedValue({ error: null }),
    ...overrides,
  }
}

vi.mock('@clerk/react', () => ({
  useAuth: vi.fn(() => ({ isSignedIn: false, isLoaded: true })),
  useSignIn: vi.fn(() => ({ signIn: makeSignIn() })),
  useSignUp: vi.fn(() => ({ signUp: makeSignUp() })),
}))

afterEach(() => {
  vi.clearAllMocks()
})

describe('SignInPage', () => {
  it('renders the value panel headline', async () => {
    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )
    expect(screen.getByText(/every job that matters/i)).toBeInTheDocument()
  })

  it('renders Sign in and Create account mode buttons', async () => {
    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /create account/i })).toBeInTheDocument()
  })

  it('renders the Continue with Google button', async () => {
    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )
    expect(screen.getByRole('button', { name: /continue with google/i })).toBeInTheDocument()
  })

  it('renders the email input', async () => {
    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )
    expect(screen.getByPlaceholderText(/you@example\.com/i)).toBeInTheDocument()
  })

  it('redirects to / when already signed in', async () => {
    const { useAuth } = await import('@clerk/react')
    // mockReturnValueOnce (not mockReturnValue) — a persistent override would
    // leak into every later test in this file since afterEach only clears
    // call history, not configured return values.
    vi.mocked(useAuth).mockReturnValueOnce({ isSignedIn: true, isLoaded: true } as never)
    const { SignInPage } = await import('../SignInPage')
    const { container } = render(
      <MemoryRouter initialEntries={['/login']}>
        <SignInPage />
      </MemoryRouter>
    )
    expect(container.querySelector('[data-testid="sign-in-panel"]')).not.toBeInTheDocument()
  })
})

describe('SignInPage email flow — sign-in mode', () => {
  it('calls signIn.emailCode.sendCode when email is submitted', async () => {
    const signIn = makeSignIn()
    const { useSignIn } = await import('@clerk/react')
    vi.mocked(useSignIn).mockReturnValue({ signIn } as never)

    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )

    fireEvent.change(screen.getByPlaceholderText(/you@example\.com/i), {
      target: { value: 'user@test.com' },
    })
    fireEvent.submit(screen.getByPlaceholderText(/you@example\.com/i).closest('form')!)

    await waitFor(() => {
      expect(signIn.emailCode.sendCode).toHaveBeenCalledWith({ emailAddress: 'user@test.com' })
    })
  })

  it('shows the OTP view after the email is submitted successfully', async () => {
    const signIn = makeSignIn()
    const { useSignIn } = await import('@clerk/react')
    vi.mocked(useSignIn).mockReturnValue({ signIn } as never)

    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )

    fireEvent.change(screen.getByPlaceholderText(/you@example\.com/i), {
      target: { value: 'user@test.com' },
    })
    fireEvent.submit(screen.getByPlaceholderText(/you@example\.com/i).closest('form')!)

    await waitFor(() => {
      expect(screen.getByText(/check your email/i)).toBeInTheDocument()
    })
  })

  it('shows an error and stays on the methods view when sendCode fails', async () => {
    const signIn = makeSignIn({
      emailCode: {
        sendCode: vi.fn().mockResolvedValue({ error: { longMessage: 'Email is invalid' } }),
        verifyCode: vi.fn(),
      },
    })
    const { useSignIn } = await import('@clerk/react')
    vi.mocked(useSignIn).mockReturnValue({ signIn } as never)

    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )

    fireEvent.change(screen.getByPlaceholderText(/you@example\.com/i), {
      target: { value: 'bad' },
    })
    fireEvent.submit(screen.getByPlaceholderText(/you@example\.com/i).closest('form')!)

    await waitFor(() => {
      expect(screen.getByText('Email is invalid')).toBeInTheDocument()
    })
    expect(screen.queryByText(/check your email/i)).not.toBeInTheDocument()
  })

  it('verifies the code and finalizes the sign-in when status is complete', async () => {
    const signIn = makeSignIn({ status: 'complete' })
    const { useSignIn } = await import('@clerk/react')
    vi.mocked(useSignIn).mockReturnValue({ signIn } as never)

    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )

    fireEvent.change(screen.getByPlaceholderText(/you@example\.com/i), {
      target: { value: 'user@test.com' },
    })
    fireEvent.submit(screen.getByPlaceholderText(/you@example\.com/i).closest('form')!)
    await waitFor(() => expect(screen.getByText(/check your email/i)).toBeInTheDocument())

    const otpInput = screen.getByPlaceholderText('000000')
    fireEvent.change(otpInput, { target: { value: '123456' } })
    fireEvent.submit(otpInput.closest('form')!)

    await waitFor(() => {
      expect(signIn.emailCode.verifyCode).toHaveBeenCalledWith({ code: '123456' })
    })
    await waitFor(() => expect(signIn.finalize).toHaveBeenCalled())
  })

  it('does not finalize when the code verification fails', async () => {
    const signIn = makeSignIn({
      status: 'needs_first_factor',
      emailCode: {
        sendCode: vi.fn().mockResolvedValue({ error: null }),
        verifyCode: vi.fn().mockResolvedValue({ error: { message: 'Incorrect code' } }),
      },
    })
    const { useSignIn } = await import('@clerk/react')
    vi.mocked(useSignIn).mockReturnValue({ signIn } as never)

    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )

    fireEvent.change(screen.getByPlaceholderText(/you@example\.com/i), {
      target: { value: 'user@test.com' },
    })
    fireEvent.submit(screen.getByPlaceholderText(/you@example\.com/i).closest('form')!)
    await waitFor(() => expect(screen.getByText(/check your email/i)).toBeInTheDocument())

    const otpInput = screen.getByPlaceholderText('000000')
    fireEvent.change(otpInput, { target: { value: '000000' } })
    fireEvent.submit(otpInput.closest('form')!)

    await waitFor(() => expect(screen.getByText('Incorrect code')).toBeInTheDocument())
    expect(signIn.finalize).not.toHaveBeenCalled()
  })
})

describe('SignInPage email flow — create-account mode', () => {
  it('calls signUp.create then signUp.verifications.sendEmailCode', async () => {
    const signUp = makeSignUp()
    const { useSignUp } = await import('@clerk/react')
    vi.mocked(useSignUp).mockReturnValue({ signUp } as never)

    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )

    fireEvent.click(screen.getByRole('button', { name: /create account/i }))
    fireEvent.change(screen.getByPlaceholderText(/you@example\.com/i), {
      target: { value: 'new@test.com' },
    })
    fireEvent.submit(screen.getByPlaceholderText(/you@example\.com/i).closest('form')!)

    await waitFor(() => {
      expect(signUp.create).toHaveBeenCalledWith({ emailAddress: 'new@test.com' })
      expect(signUp.verifications.sendEmailCode).toHaveBeenCalled()
    })
  })

  it('verifies the code and finalizes the sign-up when status is complete', async () => {
    const signUp = makeSignUp({ status: 'complete' })
    const { useSignUp } = await import('@clerk/react')
    vi.mocked(useSignUp).mockReturnValue({ signUp } as never)

    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )

    fireEvent.click(screen.getByRole('button', { name: /create account/i }))
    fireEvent.change(screen.getByPlaceholderText(/you@example\.com/i), {
      target: { value: 'new@test.com' },
    })
    fireEvent.submit(screen.getByPlaceholderText(/you@example\.com/i).closest('form')!)
    await waitFor(() => expect(screen.getByText(/check your email/i)).toBeInTheDocument())

    const otpInput = screen.getByPlaceholderText('000000')
    fireEvent.change(otpInput, { target: { value: '654321' } })
    fireEvent.submit(otpInput.closest('form')!)

    await waitFor(() => {
      expect(signUp.verifications.verifyEmailCode).toHaveBeenCalledWith({ code: '654321' })
    })
    await waitFor(() => expect(signUp.finalize).toHaveBeenCalled())
  })

  it('shows an error when signUp.create fails and does not advance to the OTP view', async () => {
    const signUp = makeSignUp({
      create: vi.fn().mockResolvedValue({ error: { longMessage: 'Email already in use' } }),
    })
    const { useSignUp } = await import('@clerk/react')
    vi.mocked(useSignUp).mockReturnValue({ signUp } as never)

    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )

    fireEvent.click(screen.getByRole('button', { name: /create account/i }))
    fireEvent.change(screen.getByPlaceholderText(/you@example\.com/i), {
      target: { value: 'dupe@test.com' },
    })
    fireEvent.submit(screen.getByPlaceholderText(/you@example\.com/i).closest('form')!)

    await waitFor(() => expect(screen.getByText('Email already in use')).toBeInTheDocument())
    expect(screen.queryByText(/check your email/i)).not.toBeInTheDocument()
    expect(signUp.verifications.sendEmailCode).not.toHaveBeenCalled()
  })
})

describe('SignInPage Google button', () => {
  it('calls signIn.sso with the Google OAuth strategy', async () => {
    const signIn = makeSignIn()
    const { useSignIn } = await import('@clerk/react')
    vi.mocked(useSignIn).mockReturnValue({ signIn } as never)

    const { SignInPage } = await import('../SignInPage')
    render(
      <MemoryRouter>
        <SignInPage />
      </MemoryRouter>
    )

    fireEvent.click(screen.getByRole('button', { name: /continue with google/i }))
    expect(signIn.sso).toHaveBeenCalledWith(expect.objectContaining({ strategy: 'oauth_google' }))
  })
})
