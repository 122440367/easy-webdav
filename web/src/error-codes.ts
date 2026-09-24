// English fallbacks for the stable machine-readable API error codes.
export const errorMessages: Record<string, string> = {
  INVALID_CREDENTIALS: 'Invalid username or password',
  UNAUTHENTICATED: 'Authentication required',
  READ_ONLY: 'This account is read-only',
  INVALID_PATH: 'The path is invalid',
  FILE_TOO_LARGE: 'The file is too large to preview',
  QUOTA_EXCEEDED: 'Storage quota exceeded',
  RATE_LIMITED: 'Too many attempts, please wait a moment',
  USER_EXISTS: 'That username is already taken',
  LAST_ADMIN: 'The last administrator cannot be removed',
  FORBIDDEN: 'You do not have permission to do that',
  SETUP_LOCKED: 'Setup has already been completed'
}

/** Maps an API error code onto an i18n message key. */
export const errorKeys: Record<string, string> = {
  INVALID_CREDENTIALS: 'errors.invalidCredentials',
  UNAUTHENTICATED: 'errors.unauthenticated',
  READ_ONLY: 'errors.readOnly',
  INVALID_PATH: 'errors.invalidPath',
  FILE_TOO_LARGE: 'errors.fileTooLarge',
  QUOTA_EXCEEDED: 'errors.quotaExceeded',
  RATE_LIMITED: 'errors.rateLimited',
  USER_EXISTS: 'errors.userExists',
  LAST_ADMIN: 'errors.lastAdmin',
  FORBIDDEN: 'errors.forbidden',
  SETUP_LOCKED: 'errors.setupLocked'
}

export function messageFor(code: string, fallback = 'Request failed') {
  return errorMessages[code] || fallback
}

export function errorKey(code?: string) {
  if (code && errorKeys[code]) return errorKeys[code]
  return 'errors.generic'
}
