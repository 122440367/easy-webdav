export type ConnectInfo = {
  origin: string
  davURL: string
  insecure: boolean
  username: string
  /** UNC path accepted by Windows Explorer's "Map network drive". */
  windowsPath: string
  /** Command that mounts the share read-write on Linux with davfs2. */
  davfsCommand: string
  /** Interactive rclone remote creation. */
  rcloneCreateCommand: string
  rcloneVerifyCommand: string
  netUseCommand: string
}

function normalizeBase(value: string) {
  const trimmed = (value || '/').trim()
  const withLeading = trimmed.startsWith('/') ? trimmed : `/${trimmed}`
  const collapsed = withLeading.replace(/\/{2,}/g, '/')
  return collapsed.endsWith('/') ? collapsed : `${collapsed}/`
}

/**
 * Derives every snippet shown by the "connect" dialog from the address the
 * panel is currently served from, so a deployment behind a sub-path keeps
 * working without extra configuration.
 */
export function buildConnectInfo(input: { origin: string; baseURL: string; username: string; insecure?: boolean }): ConnectInfo {
  const origin = input.origin.replace(/\/+$/, '')
  const base = normalizeBase(input.baseURL)
  const davURL = `${origin}${base}dav/`
  const parsed = new URL(davURL)
  const secure = parsed.protocol === 'https:'
  const port = parsed.port || (secure ? '443' : '80')
  const share = `${base}dav`.replace(/\//g, '\\')
  const windowsPath = secure ? `\\\\${parsed.hostname}@SSL@${port}${share}` : `\\\\${parsed.hostname}@${port}${share}`
  const host = parsed.hostname
  return {
    origin,
    davURL,
    insecure: input.insecure ?? !secure,
    username: input.username,
    windowsPath,
    davfsCommand: `sudo mount -t davfs ${davURL} /mnt/${host} -o uid=$(id -u)`,
    rcloneCreateCommand: `rclone config create easy-webdav webdav url=${davURL} vendor=other user=${input.username}`,
    rcloneVerifyCommand: 'rclone lsd easy-webdav:',
    netUseCommand: `net use Z: ${davURL} /user:${input.username} *`
  }
}

/** Reads the panel's own base href so sub-path deployments resolve correctly. */
export function currentBaseURL() {
  if (typeof document === 'undefined') return '/'
  return document.querySelector('base')?.getAttribute('href') || '/'
}
