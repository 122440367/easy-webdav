export const errorMessages: Record<string, string> = {
  INVALID_CREDENTIALS: 'Invalid username or password',
  UNAUTHENTICATED: 'Authentication required',
  READ_ONLY: 'This account is read-only',
  INVALID_PATH: 'The path is invalid',
  FILE_TOO_LARGE: 'The file is too large to preview',
  QUOTA_EXCEEDED: 'Storage quota exceeded'
}
export function messageFor(code: string, fallback = 'Request failed') { return errorMessages[code] || fallback }
