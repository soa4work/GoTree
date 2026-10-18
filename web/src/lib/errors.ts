import type { TFunction } from 'i18next'
import { ApiError, type ErrorDetail } from '../api/client'

// Keys are built from server codes at runtime, so they cannot be checked against the typed resources.
type LooseT = (key: string, options?: Record<string, unknown>) => string

function translate(t: TFunction, prefix: string, detail: ErrorDetail, fallback: string): string {
  if (!detail.code) return fallback
  const loose = t as unknown as LooseT
  const params = { ...detail.params }
  // Enum words in "must be one of" lists have their own translations (apiError.option.<word>).
  if (typeof params.options === 'string') {
    params.options = params.options
      .split(', ')
      .map((word) => loose(`apiError.option.${word}`, { defaultValue: word }))
      .join(', ')
  }
  // A numeric "count" param selects the plural form (_one/_few/_many/_other).
  return loose(`${prefix}.${detail.code}`, { ...params, defaultValue: fallback })
}

/**
 * The text of an error in the interface language. Server errors are looked up
 * by their code under `apiError.<code>`; an unknown code, an old server or any
 * other error shows the message as it came.
 */
export function errorText(err: unknown, t: TFunction): string {
  if (err instanceof ApiError) return translate(t, 'apiError', err, err.message)
  if (err instanceof Error) return err.message
  return String(err)
}

/** Validation messages of a 422 by field name, translated by `apiError.field.<code>`. */
export function fieldErrors(error: unknown, t: TFunction): Record<string, string> {
  if (!(error instanceof ApiError)) return {}
  return Object.fromEntries(
    Object.entries(error.fields).map(([field, message]) => {
      const detail = error.fieldCodes[field] ?? {}
      const text = translate(t, 'apiError.field', detail, message)
      // A row of a transcription names its column: "givenNames: is required".
      const column = detail.code ? detail.params?.field : undefined
      return [field, typeof column === 'string' ? `${column}: ${text}` : text]
    }),
  )
}

/** Field errors from a 422, and a form-level message for anything else. */
export function formErrors(error: unknown, t: TFunction, fallback: (message: string) => string) {
  const fields = fieldErrors(error, t)
  if (Object.keys(fields).length > 0) return { fields, form: null }
  if (error instanceof Error) return { fields, form: fallback(errorText(error, t)) }
  return { fields, form: null }
}
