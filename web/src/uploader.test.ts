import { afterEach, describe, expect, it, vi } from 'vitest'
import { CHUNK_SIZE } from './upload-state'
import { createTask, runUpload } from './uploader'

type Call = { url: string; method: string }

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => JSON.stringify(body)
  } as unknown as Response
}

function installFetch(options: { failChunkOnce?: number } = {}) {
  const calls: Call[] = []
  let inFlight = 0
  let maxInFlight = 0
  const failed = new Set<number>()
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = String(input)
    const method = (init.method || 'GET').toUpperCase()
    calls.push({ url, method })
    if (url.includes('/uploads?') || url.endsWith('/uploads')) {
      return jsonResponse({ id: 'session-1' }, 201)
    }
    const chunk = /\/chunks\/(\d+)/.exec(url)
    if (chunk) {
      inFlight += 1
      maxInFlight = Math.max(maxInFlight, inFlight)
      await new Promise((resolve) => setTimeout(resolve, 5))
      inFlight -= 1
      const index = Number(chunk[1])
      if (options.failChunkOnce === index && !failed.has(index)) {
        failed.add(index)
        return jsonResponse({ message: 'boom' }, 502)
      }
      return jsonResponse({ written: 1 })
    }
    return jsonResponse({ path: '/files/big.bin' })
  })
  vi.stubGlobal('fetch', fetchMock)
  return { calls, maxInFlight: () => maxInFlight }
}

describe('chunked uploader', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('uploads every chunk, keeps the concurrency bound and completes', async () => {
    const { calls, maxInFlight } = installFetch()
    const file = new File([new Uint8Array(CHUNK_SIZE * 3 + 10)], 'big.bin')
    const task = createTask(file, 'big.bin', 'rename')
    await runUpload(task, { directory: 'incoming', userId: '7' })

    expect(task.state.status).toBe('complete')
    expect(task.state.completed).toBe(4)
    expect(maxInFlight()).toBeLessThanOrEqual(3)
    expect(calls.filter((call) => call.url.includes('/chunks/')).length).toBe(4)
    expect(calls[0].url).toContain('user_id=7')
    expect(calls.some((call) => call.url.includes('/complete'))).toBe(true)
  })

  it('retries a failing chunk without restarting the file', async () => {
    const { calls } = installFetch({ failChunkOnce: 1 })
    const file = new File([new Uint8Array(CHUNK_SIZE * 2)], 'retry.bin')
    const task = createTask(file, 'retry.bin', 'overwrite')
    await runUpload(task, {})

    expect(task.state.status).toBe('complete')
    const chunkOne = calls.filter((call) => call.url.endsWith('/chunks/1')).length
    expect(chunkOne).toBe(2)
    expect(calls.filter((call) => call.url.endsWith('/chunks/0')).length).toBe(1)
  })

  it('marks the file as failed after exhausting the retry budget', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.includes('/chunks/')) return jsonResponse({ message: 'offline' }, 503)
      if (url.endsWith('/uploads') || url.includes('/uploads?')) return jsonResponse({ id: 'session-2' }, 201)
      return jsonResponse({ message: 'offline' }, 503)
    }))
    const task = createTask(new File([new Uint8Array(8)], 'small.bin'), 'small.bin', 'overwrite')
    await expect(runUpload(task)).rejects.toBeTruthy()
    expect(task.state.status).toBe('failed')
  })

  it('reports quota errors raised while creating the session', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ code: 'QUOTA_EXCEEDED', message: 'quota exceeded', details: { remaining: 12 } }, 413)))
    const task = createTask(new File([new Uint8Array(8)], 'small.bin'), 'small.bin', 'overwrite')
    await expect(runUpload(task)).rejects.toMatchObject({ code: 'QUOTA_EXCEEDED' })
    expect(task.state.status).toBe('failed')
  })
})
