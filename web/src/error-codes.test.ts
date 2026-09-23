import { describe, expect, it } from 'vitest'
import { messageFor } from './error-codes'
describe('error messages', () => { it('maps stable codes and falls back', () => { expect(messageFor('INVALID_PATH')).toContain('path'); expect(messageFor('UNKNOWN', 'fallback')).toBe('fallback') }) })
