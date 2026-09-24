import { describe, expect, it } from 'vitest'
import { buildConnectInfo } from './connect'

describe('connect snippets', () => {
  it('builds the root deployment snippets', () => {
    const info = buildConnectInfo({ origin: 'https://nas:8443', baseURL: '/', username: 'alice' })
    expect(info.davURL).toBe('https://nas:8443/dav/')
    expect(info.insecure).toBe(false)
    expect(info.windowsPath).toBe('\\\\nas@SSL@8443\\dav')
    expect(info.netUseCommand).toBe('net use Z: https://nas:8443/dav/ /user:alice *')
    expect(info.rcloneCreateCommand).toContain('url=https://nas:8443/dav/')
    expect(info.rcloneCreateCommand).toContain('user=alice')
    expect(info.davfsCommand).toContain('https://nas:8443/dav/')
  })

  it('keeps sub-path deployments working', () => {
    const info = buildConnectInfo({ origin: 'https://host', baseURL: '/files/', username: 'bob' })
    expect(info.davURL).toBe('https://host/files/dav/')
    expect(info.windowsPath).toBe('\\\\host@SSL@443\\files\\dav')
  })

  it('normalises a base without slashes and flags plain http', () => {
    const info = buildConnectInfo({ origin: 'http://host', baseURL: 'files', username: 'carol' })
    expect(info.davURL).toBe('http://host/files/dav/')
    expect(info.windowsPath).toBe('\\\\host@80\\files\\dav')
    expect(info.insecure).toBe(true)
  })

  it('honours an explicit insecure override', () => {
    const info = buildConnectInfo({ origin: 'http://host:8080', baseURL: '/', username: 'dan', insecure: false })
    expect(info.insecure).toBe(false)
  })
})
