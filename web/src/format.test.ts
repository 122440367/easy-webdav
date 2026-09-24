import { describe, expect, it } from 'vitest'
import { formatSize, quotaToBytes, splitQuota, usagePercent } from './format'

describe('formatting helpers', () => {
  it('renders human readable sizes', () => {
    expect(formatSize(0)).toBe('0 B')
    expect(formatSize(512)).toBe('512 B')
    expect(formatSize(1536)).toBe('1.5 KB')
    expect(formatSize(5 * 1024 * 1024)).toBe('5.0 MB')
  })

  it('round-trips quotas through human readable units', () => {
    expect(splitQuota(10 * 1024 * 1024 * 1024)).toEqual({ amount: 10, unit: 1024 * 1024 * 1024 })
    expect(quotaToBytes(10, 1024 * 1024 * 1024)).toBe(10737418240)
    expect(quotaToBytes(0, 1024 * 1024 * 1024)).toBe(0)
  })

  it('computes usage percentages without dividing by zero', () => {
    expect(usagePercent(5, 0)).toBe(0)
    expect(usagePercent(5, 10)).toBe(50)
    expect(usagePercent(20, 10)).toBe(100)
  })
})
