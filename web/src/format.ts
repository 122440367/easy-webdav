const UNITS = ['B', 'KB', 'MB', 'GB', 'TB']

/** Human readable byte count used across the panel. */
export function formatSize(value = 0) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  let amount = value
  let index = 0
  while (amount >= 1024 && index < UNITS.length - 1) {
    amount /= 1024
    index += 1
  }
  return `${index === 0 ? amount : amount.toFixed(1)} ${UNITS[index]}`
}

export const QUOTA_UNITS = [
  { label: 'KB', bytes: 1024 },
  { label: 'MB', bytes: 1024 * 1024 },
  { label: 'GB', bytes: 1024 * 1024 * 1024 }
]

/** Splits a byte quota into the largest unit that keeps the number readable. */
export function splitQuota(bytes: number) {
  if (!bytes || bytes <= 0) return { amount: 0, unit: QUOTA_UNITS[1].bytes }
  for (let index = QUOTA_UNITS.length - 1; index >= 0; index -= 1) {
    if (bytes % QUOTA_UNITS[index].bytes === 0) return { amount: bytes / QUOTA_UNITS[index].bytes, unit: QUOTA_UNITS[index].bytes }
  }
  return { amount: Math.round((bytes / QUOTA_UNITS[1].bytes) * 10) / 10, unit: QUOTA_UNITS[1].bytes }
}

export function quotaToBytes(amount: number | string, unit: number) {
  const value = typeof amount === 'string' ? Number(amount) : amount
  if (!Number.isFinite(value) || value <= 0) return 0
  return Math.round(value * unit)
}

export function formatDate(value: string | number | Date) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString()
}

export function usagePercent(used = 0, quota = 0) {
  if (!quota) return 0
  return Math.min(100, Math.round((used / quota) * 100))
}
