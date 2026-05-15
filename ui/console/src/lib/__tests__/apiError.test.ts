import { describe, it, expect } from 'vitest'
import { ApiValidationError, parseApiError } from '@/lib/api-error'

// ─────────────────────────────────────────────────────────────────────────────
// ApiValidationError
// ─────────────────────────────────────────────────────────────────────────────
describe('ApiValidationError', () => {
  it('is an instance of Error', () => {
    const err = new ApiValidationError([])
    expect(err).toBeInstanceOf(Error)
  })

  it('has name "ApiValidationError"', () => {
    const err = new ApiValidationError([])
    expect(err.name).toBe('ApiValidationError')
  })

  it('has default message "Validation failed"', () => {
    const err = new ApiValidationError([])
    expect(err.message).toBe('Validation failed')
  })

  it('fieldErrors() returns a field → message map', () => {
    const err = new ApiValidationError([
      { field: 'email', code: 'required', message: 'Email is required' },
      { field: 'display_name', code: 'too_long', message: 'Display name is too long' },
    ])
    expect(err.fieldErrors()).toEqual({
      email: 'Email is required',
      display_name: 'Display name is too long',
    })
  })

  it('fieldErrors() returns empty object when there are no fields', () => {
    const err = new ApiValidationError([])
    expect(err.fieldErrors()).toEqual({})
  })

  it('fieldErrors() uses the last entry when a field appears multiple times', () => {
    // Object.fromEntries preserves last-write semantics for duplicate keys
    const err = new ApiValidationError([
      { field: 'email', code: 'required', message: 'Email is required' },
      { field: 'email', code: 'invalid', message: 'Email is invalid' },
    ])
    expect(err.fieldErrors()['email']).toBe('Email is invalid')
  })

  it('exposes the raw fields array', () => {
    const fields = [{ field: 'title', code: 'required', message: 'Title is required' }]
    const err = new ApiValidationError(fields)
    expect(err.fields).toEqual(fields)
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// parseApiError
// ─────────────────────────────────────────────────────────────────────────────
function makeJsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function makeTextResponse(status: number, text: string): Response {
  return new Response(text, { status, headers: { 'Content-Type': 'text/plain' } })
}

describe('parseApiError', () => {
  it('returns ApiValidationError when body has a non-empty errors array', async () => {
    const res = makeJsonResponse(422, {
      errors: [{ field: 'email', code: 'required', message: 'Email is required' }],
    })
    const err = await parseApiError(res, 'fallback')
    expect(err).toBeInstanceOf(ApiValidationError)
    expect((err as ApiValidationError).fieldErrors()).toEqual({
      email: 'Email is required',
    })
  })

  it('returns plain Error from body.error.message', async () => {
    const res = makeJsonResponse(400, { error: { message: 'Bad request' } })
    const err = await parseApiError(res, 'fallback')
    expect(err).toBeInstanceOf(Error)
    expect(err).not.toBeInstanceOf(ApiValidationError)
    expect(err.message).toBe('Bad request')
  })

  it('falls back to the fallback string when body has no recognized error shape', async () => {
    const res = makeJsonResponse(500, { something: 'unexpected' })
    const err = await parseApiError(res, 'Request failed: 500')
    expect(err.message).toBe('Request failed: 500')
  })

  it('falls back to fallback when body is not valid JSON', async () => {
    const res = makeTextResponse(500, 'Internal Server Error')
    const err = await parseApiError(res, 'Request failed: 500')
    expect(err.message).toBe('Request failed: 500')
  })

  it('falls back to fallback when errors array is empty', async () => {
    const res = makeJsonResponse(422, { errors: [] })
    const err = await parseApiError(res, 'Validation failed')
    expect(err.message).toBe('Validation failed')
    expect(err).not.toBeInstanceOf(ApiValidationError)
  })

  it('does not crash on completely empty body', async () => {
    const res = new Response('', { status: 500 })
    const err = await parseApiError(res, 'fallback')
    expect(err.message).toBe('fallback')
  })

  it('prefers errors array over error.message when both are present', async () => {
    const res = makeJsonResponse(422, {
      errors: [{ field: 'name', code: 'required', message: 'Name required' }],
      error: { message: 'Generic error' },
    })
    const err = await parseApiError(res, 'fallback')
    expect(err).toBeInstanceOf(ApiValidationError)
  })
})
