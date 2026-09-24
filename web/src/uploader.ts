import { ApiError, endpoint, request } from './api'
import { CHUNK_SIZE, MAX_ATTEMPTS, UPLOAD_CONCURRENCY, chunkCount, finishUpload, markChunk, markRetry, startUpload, type UploadState } from './upload-state'

export type ConflictChoice = 'overwrite' | 'skip' | 'rename'

export type UploadTask = {
  file: File
  /** Path relative to the browsed directory, keeping folder upload structure. */
  relativePath: string
  conflict: ConflictChoice
  state: UploadState
  error?: string
}

export function createTask(file: File, relativePath: string, conflict: ConflictChoice): UploadTask {
  return { file, relativePath, conflict, state: startUpload(chunkCount(file.size)) }
}

type Handlers = { onProgress?: (task: UploadTask) => void; userId?: string; directory?: string }

function withUser(path: string, userId?: string) {
  return userId ? `${path}${path.includes('?') ? '&' : '?'}user_id=${encodeURIComponent(userId)}` : path
}

/**
 * Uploads one file in fixed-size chunks. Chunks run with a small amount of
 * parallelism and each failed chunk is retried a few times before the file is
 * marked as failed; already accepted chunks are never re-uploaded.
 */
export async function runUpload(task: UploadTask, handlers: Handlers = {}) {
  const { file, relativePath, conflict, state } = task
  const remotePath = [handlers.directory, relativePath].filter(Boolean).join('/')
  const notify = () => handlers.onProgress?.(task)
  Object.assign(state, startUpload(state.total), { status: 'uploading' })
  notify()

  let session: { id: string }
  try {
    session = await request<{ id: string }>(withUser('api/v1/uploads', handlers.userId), {
      method: 'POST',
      body: JSON.stringify({ path: remotePath, total_size: file.size, conflict })
    })
  } catch (error) {
    Object.assign(state, { status: 'failed' })
    task.error = error instanceof ApiError ? error.message : String(error)
    notify()
    throw error
  }

  const chunks = Array.from({ length: chunkCount(file.size) }, (_value, index) => index)
  const queue = [...chunks]
  let failure: unknown = null
  const worker = async () => {
    while (queue.length && !failure) {
      const index = queue.shift() as number
      const part = file.slice(index * CHUNK_SIZE, Math.min(file.size, (index + 1) * CHUNK_SIZE))
      for (let attempt = 1; ; attempt += 1) {
        try {
          await request(withUser(`api/v1/uploads/${session.id}/chunks/${index}`, handlers.userId), { method: 'PUT', body: part })
          Object.assign(state, markChunk(state))
          notify()
          break
        } catch (error) {
          Object.assign(state, markRetry(state))
          notify()
          if (attempt >= MAX_ATTEMPTS) {
            failure = error
            return
          }
        }
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(UPLOAD_CONCURRENCY, chunks.length) }, worker))
  if (failure) {
    Object.assign(state, { status: 'failed' })
    task.error = failure instanceof ApiError ? failure.message : String(failure)
    notify()
    throw failure
  }
  try {
    await request(withUser(`api/v1/uploads/${session.id}/complete`, handlers.userId), { method: 'POST' })
  } catch (error) {
    Object.assign(state, { status: 'failed' })
    task.error = error instanceof ApiError ? error.message : String(error)
    notify()
    throw error
  }
  Object.assign(state, finishUpload(state))
  notify()
  return endpoint(withUser(`api/v1/uploads/${session.id}`, handlers.userId))
}
