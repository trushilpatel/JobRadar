export interface ValidationFieldError {
  field: string
  code: string
  message: string
}

export class ApiValidationError extends Error {
  readonly fields: ValidationFieldError[]

  constructor(fields: ValidationFieldError[]) {
    super('Validation failed')
    this.name = 'ApiValidationError'
    this.fields = fields
  }

  // Returns a Record<field, message> for easy field lookup in forms
  fieldErrors(): Record<string, string> {
    return Object.fromEntries(this.fields.map(f => [f.field, f.message]))
  }
}

// Parses a non-ok response into either ApiValidationError or a plain Error
export async function parseApiError(res: Response, fallback: string): Promise<Error> {
  try {
    const body = await res.json()
    if (Array.isArray(body.errors) && body.errors.length > 0) {
      return new ApiValidationError(body.errors)
    }
    if (body.error?.message) {
      return new Error(body.error.message)
    }
  } catch {
    // ignore JSON parse errors
  }
  return new Error(fallback)
}
