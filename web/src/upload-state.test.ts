import { describe, expect, it } from 'vitest'
import { finishUpload, markChunk, markRetry, startUpload } from './upload-state'
describe('upload state', () => { it('completes after all chunks', () => { let state=startUpload(2); state=markChunk(state); state=markChunk(state); expect(finishUpload(state).status).toBe('complete') }); it('fails after three retries', () => { let state=startUpload(1); state=markRetry(state);state=markRetry(state);state=markRetry(state);expect(state.status).toBe('failed') }) })
