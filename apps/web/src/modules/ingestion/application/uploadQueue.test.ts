import { UploadQueue } from './uploadQueue'
import type { UploadCallbacks, UploadSpec } from './ports'

describe('UploadQueue', () => {
  it('runs two uploads and leaves the rest queued', () => {
    const callbacks: UploadCallbacks[] = []
    const ports = {
      upload: vi.fn((_file: File, _spec: UploadSpec, cb: UploadCallbacks) => {
        callbacks.push(cb)
        return { abort: vi.fn() }
      }),
      cancelUpload: vi.fn(() => Promise.resolve()),
    }
    const queue = new UploadQueue(ports)
    const spec = { batchId: 'batch', relation: 'none' as const, fileName: 'video.mkv' }
    queue.add(
      [1, 2, 3].map((index) => ({
        file: new File([String(index)], `video-${String(index)}.mkv`),
        spec,
      })),
    )
    expect(ports.upload).toHaveBeenCalledTimes(2)
    expect(queue.getSnapshot().map((item) => item.state)).toEqual([
      'uploading',
      'uploading',
      'queued',
    ])
    callbacks[0]?.onSuccess()
    expect(ports.upload).toHaveBeenCalledTimes(3)
    expect(queue.getSnapshot().map((item) => item.state)).toEqual([
      'finished',
      'uploading',
      'uploading',
    ])
  })
})
