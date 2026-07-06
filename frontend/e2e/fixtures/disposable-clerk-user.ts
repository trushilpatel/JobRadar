import { randomUUID } from 'node:crypto'

// Creates and tears down throwaway Clerk identities for E2E tests that need
// to exercise a genuinely destructive flow (self-delete account, admin-delete
// user) without ever touching the shared TEST_ROLE_USER / TEST_ROLE_ADMIN
// accounts every other spec depends on.
//
// Uses the `+clerk_test@` email convention (same as TEST_ROLE_USER_EMAIL /
// TEST_ROLE_ADMIN_EMAIL) so sign-in through the real UI hits Clerk's test-mode
// fixed OTP (424242) instead of sending a real email.

export interface DisposableClerkUser {
  clerkUserId: string
  email: string
  password: string
  otp: string
}

function requireEnv(name: string): string {
  const value = process.env[name]
  if (!value) throw new Error(`Missing ${name} in env — run via "npm run test:e2e"`)
  return value
}

async function clerkRequest(path: string, init: RequestInit) {
  const secretKey = requireEnv('CLERK_SECRET_KEY')
  const res = await fetch(`https://api.clerk.com/v1${path}`, {
    ...init,
    headers: {
      Authorization: `Bearer ${secretKey}`,
      'Content-Type': 'application/json',
      ...init.headers,
    },
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`Clerk API ${init.method ?? 'GET'} ${path} failed: ${res.status} ${body}`)
  }
  return res.status === 204 ? null : ((await res.json()) as unknown)
}

// A fixed OTP only applies to Clerk test-mode emails tagged `+clerk_test@`.
const DISPOSABLE_OTP = '424242'
const DISPOSABLE_PASSWORD = 'JobRadar!E2EDisposable2026#'

export async function createDisposableClerkUser(
  role: 'user' | 'admin' = 'user'
): Promise<DisposableClerkUser> {
  const email = `e2e+clerk_test+${randomUUID().slice(0, 8)}@example.com`
  const created = (await clerkRequest('/users', {
    method: 'POST',
    body: JSON.stringify({
      email_address: [email],
      password: DISPOSABLE_PASSWORD,
      public_metadata: { role },
      skip_password_checks: true,
    }),
  })) as { id: string }

  return { clerkUserId: created.id, email, password: DISPOSABLE_PASSWORD, otp: DISPOSABLE_OTP }
}

// Safety-net cleanup — call in test.afterAll even when the spec's own UI flow
// is expected to have already deleted the account, since afterAll should
// still run (and clean up) if an earlier assertion throws mid-test. Clerk
// returns 404 for an already-deleted user; treat that as success.
export async function deleteDisposableClerkUser(clerkUserId: string): Promise<void> {
  try {
    await clerkRequest(`/users/${clerkUserId}`, { method: 'DELETE' })
  } catch (err) {
    if (err instanceof Error && err.message.includes('404')) return
    throw err
  }
}
