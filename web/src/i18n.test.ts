import { describe, expect, it } from 'vitest'
import { messages } from './i18n'

function flat(value: Record<string, unknown>, prefix = ''): string[] {
  return Object.entries(value).flatMap(([key, item]) =>
    item && typeof item === 'object' && !Array.isArray(item) ? flat(item as Record<string, unknown>, `${prefix}${key}.`) : [`${prefix}${key}`]
  )
}

describe('translations', () => {
  it('keeps both locales in sync', () => {
    const english = flat(messages.en).sort()
    const chinese = flat(messages['zh-CN']).sort()
    expect(chinese).toEqual(english)
  })

  it('covers every API error code', () => {
    const keys = flat(messages.en)
    for (const key of [
      'errors.invalidCredentials',
      'errors.unauthenticated',
      'errors.readOnly',
      'errors.invalidPath',
      'errors.fileTooLarge',
      'errors.quotaExceeded',
      'errors.rateLimited',
      'errors.userExists',
      'errors.lastAdmin',
      'errors.forbidden',
      'errors.setupLocked'
    ]) {
      expect(keys).toContain(key)
    }
  })
})
