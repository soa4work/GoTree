import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { ApiError, NetworkError } from '../api/client'
import i18n from '../i18n'
import { loggedInState, mockApi, renderApp } from '../test/render'
import { errorText, fieldErrors, formErrors } from './errors'

const t = i18n.t.bind(i18n)

describe('errorText', () => {
  it('translates a known code', () => {
    const err = new ApiError(404, 'not found', {}, { code: 'not_found' })
    expect(errorText(err, t)).toBe('Not found.')
  })

  it('interpolates the params', () => {
    const err = new ApiError(413, 'the file is larger than 8 MB', {}, { code: 'file_too_large', params: { limitMB: 8 } })
    expect(errorText(err, t)).toBe('The file is larger than 8 MB.')
  })

  it('falls back to the server message for an unknown code or none', () => {
    expect(errorText(new ApiError(400, 'something new', {}, { code: 'brand_new_code' }), t)).toBe('something new')
    expect(errorText(new Error('boom'), t)).toBe('boom')
  })

  it('translates a response without a code and a request that got no answer', () => {
    expect(errorText(new ApiError(502, 'Bad Gateway'), t)).toBe('The server is not available right now (error 502). Try again in a moment.')
    expect(errorText(new ApiError(404, 'Not Found'), t)).toBe('The server answered with error 404.')
    for (const err of [
      new NetworkError('Failed to fetch'),
      new TypeError('Failed to fetch'),
      new TypeError('Load failed'),
      new TypeError('NetworkError when attempting to fetch resource.'),
    ]) {
      expect(errorText(err, t)).toBe('No connection to the server. Check the connection and try again.')
    }
    // A bug in the code stays visible.
    expect(errorText(new TypeError("Cannot read properties of undefined (reading 'x')"), t)).toBe("Cannot read properties of undefined (reading 'x')")
  })

  it('names the field of a transcription row error and hides an unknown key', () => {
    const row = (field: string) =>
      new ApiError(422, 'x', { 'rows.0': `${field}: is required` }, { code: 'invalid_input', fieldCodes: { 'rows.0': { code: 'required', params: { field } } } })
    expect(fieldErrors(row('givenNames'), t)['rows.0']).toBe('Given names: is required')
    expect(fieldErrors(row('weird'), t)['rows.0']).toBe('is required')
  })

  it('translates field messages and keeps unknown ones', () => {
    const err = new ApiError(
      422,
      'invalid input',
      { sex: 'must be M, F, U or X', notes: 'is odd' },
      { code: 'invalid_input', fieldCodes: { sex: { code: 'invalid_choice', params: { options: 'M, F, U, X' } }, notes: { code: 'invalid' } } },
    )
    expect(fieldErrors(err, t)).toEqual({ sex: 'must be one of: M, F, U, X', notes: 'is odd' })
    expect(formErrors(err, t, (m) => `failed: ${m}`).form).toBeNull()
    expect(formErrors(new Error('boom'), t, (m) => `failed: ${m}`).form).toBe('failed: boom')
  })
})

describe('translated server errors in the interface', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('ru')
  })
  afterAll(async () => {
    await i18n.changeLanguage('en')
  })

  it('shows a failed import in Russian, with the limit', async () => {
    mockApi({
      'GET /api/persons': [200, { items: [], total: 0 }],
      'GET /api/auth/state': [200, { ...loggedInState, tree: { id: 1, name: 'Weber', role: 'owner' } }],
      'POST /api/import/gedcom': [413, { error: 'the GEDCOM file is too large (over 400 MB)', code: 'gedcom_too_large', params: { limitMB: 400 } }],
    })
    renderApp('/import-export')
    await userEvent.upload(await screen.findByLabelText('Файл GEDCOM'), new File(['0 HEAD'], 'tree.ged'))
    await userEvent.click(await screen.findByRole('button', { name: 'Импортировать' }))
    expect(await screen.findByText('Не удалось выполнить импорт: Файл GEDCOM слишком большой (больше 400 МБ).')).toBeInTheDocument()
  })

  it('translates the option words of a choice error', () => {
    const err = new ApiError(422, 'x', { priority: 'x' }, { fieldCodes: { priority: { code: 'invalid_choice', params: { options: 'low, normal, high' } } } })
    expect(fieldErrors(err, t).priority).toBe('должно быть одним из: низкий, обычный, высокий')
  })

  it('uses Russian plural forms', () => {
    const field = (count: number) =>
      fieldErrors(new ApiError(422, 'x', { password: 'x' }, { fieldCodes: { password: { code: 'password_too_short', params: { count } } } }), t).password
    expect(field(1)).toBe('должен содержать не менее 1 символа')
    expect(field(10)).toBe('должен содержать не менее 10 символов')
  })

  it('translates a transcription row error and keeps its column', () => {
    const err = new ApiError(
      422,
      'invalid input',
      { 'rows.2': 'givenNames: is too long' },
      { code: 'invalid_input', fieldCodes: { 'rows.2': { code: 'too_long', params: { field: 'givenNames' } } } },
    )
    expect(fieldErrors(err, t)['rows.2']).toBe('Имя и отчество: слишком длинное значение')
  })

  it('shows a proxy error in Russian instead of its English status text', () => {
    expect(errorText(new ApiError(502, 'Bad Gateway'), t)).toBe('Сервер сейчас недоступен (ошибка 502). Попробуйте чуть позже.')
    expect(errorText(new NetworkError('Failed to fetch'), t)).toBe('Нет связи с сервером. Проверьте подключение и попробуйте снова.')
  })

  it('translates a GEDCOM file error with its line, without the English detail', () => {
    const err = new ApiError(
      422,
      'this does not look like a GEDCOM file (line 1)',
      {},
      { code: 'gedcom_not_gedcom', params: { line: 1, detail: 'this does not look like a GEDCOM file (line 1)' } },
    )
    expect(errorText(err, t)).toBe('Это не похоже на файл GEDCOM (строка 1).')
    expect(errorText(new ApiError(400, 'invalid JSON: EOF', {}, { code: 'invalid_json', params: { detail: 'EOF' } }), t)).toBe(
      'Запрос не является корректным JSON.',
    )
  })
})
