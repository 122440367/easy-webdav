export type UploadStatus = 'queued' | 'uploading' | 'retrying' | 'complete' | 'failed'
export type UploadState = { status: UploadStatus; completed: number; total: number; attempts: number }
export function startUpload(total: number): UploadState { return { status: 'queued', completed: 0, total, attempts: 0 } }
export function markChunk(state: UploadState): UploadState { return { ...state, status: 'uploading', completed: Math.min(state.total, state.completed + 1) } }
export function markRetry(state: UploadState): UploadState { const attempts=state.attempts+1; return { ...state, attempts, status: attempts >= 3 ? 'failed' : 'retrying' } }
export function finishUpload(state: UploadState): UploadState { return { ...state, status: state.completed === state.total ? 'complete' : 'failed' } }
