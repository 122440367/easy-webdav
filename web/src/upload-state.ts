export type UploadStatus = 'queued' | 'uploading' | 'retrying' | 'complete' | 'failed'
export type UploadState = { status: UploadStatus; completed: number; total: number; attempts: number }

/** Chunk size negotiated with the backend (see design D7). */
export const CHUNK_SIZE = 8 * 1024 * 1024
/** Chunks are uploaded with a small amount of parallelism. */
export const UPLOAD_CONCURRENCY = 3
/** A chunk is retried this many times before the file is marked failed. */
export const MAX_ATTEMPTS = 3

export function startUpload(total: number): UploadState {
  return { status: 'queued', completed: 0, total, attempts: 0 }
}

export function markChunk(state: UploadState): UploadState {
  return { ...state, status: 'uploading', completed: Math.min(state.total, state.completed + 1), attempts: 0 }
}

export function markRetry(state: UploadState): UploadState {
  const attempts = state.attempts + 1
  return { ...state, attempts, status: attempts >= MAX_ATTEMPTS ? 'failed' : 'retrying' }
}

export function finishUpload(state: UploadState): UploadState {
  return { ...state, status: state.completed === state.total ? 'complete' : 'failed' }
}

export function progress(state: UploadState) {
  if (!state.total) return state.status === 'complete' ? 100 : 0
  return Math.min(100, Math.round((state.completed / state.total) * 100))
}

export function chunkCount(size: number) {
  return Math.max(1, Math.ceil(size / CHUNK_SIZE))
}
