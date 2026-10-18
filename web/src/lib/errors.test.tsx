import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { ApiError } from '../api/client'
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
    expect(errorText(new ApiError(502, 'Bad Gateway'), t)).toBe('Bad Gateway')
    expect(errorText(new Error('network down'), t)).toBe('network down')
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

  it('uses Russian plural forms', () => {
    const field = (count: number) =>
      fieldErrors(new ApiError(422, 'x', { password: 'x' }, { fieldCodes: { password: { code: 'password_too_short', params: { count } } } }), t).password
    expect(field(1)).toBe('должен содержать не менее 1 символа')
    expect(field(10)).toBe('должен содержать не менее 10 символов')
  })
})
