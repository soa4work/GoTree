import { afterEach, describe, expect, it } from 'vitest'
import i18n from '../i18n'
import { treeDisplayName } from './treeName'

afterEach(() => i18n.changeLanguage('en'))

describe('treeDisplayName', () => {
  it('translates the default name and leaves a chosen one alone', async () => {
    await i18n.changeLanguage('ru')
    expect(treeDisplayName('Family tree', i18n.t)).toBe('Семейное дерево')
    expect(treeDisplayName('Weber family', i18n.t)).toBe('Weber family')
    await i18n.changeLanguage('kk')
    expect(treeDisplayName('Family tree', i18n.t)).toBe('Шежіре')
  })
})
