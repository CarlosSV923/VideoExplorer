import { buildUploadRows, canCancelJob, canRetryJob, multipartPartsTotal } from './uploadLogic'
import type { LocalUpload, UploadJob } from '../domain/types'

const job = (status: UploadJob['status'], error?: UploadJob['error']): UploadJob => ({
  id: status,
  batch: { id: 'b', name: '', relation: 'none', createdAt: '2026-01-01T00:00:00Z' },
  fileName: `${status}.mkv`,
  relativePath: '',
  size: 100,
  received: 20,
  progress: 34,
  status,
  ...(error ? { error } : {}),
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
})

describe('upload application logic', () => {
  it.each<[UploadJob['status'], boolean]>([
    ['uploading', true],
    ['uploaded', true],
    ['waiting_parts', true],
    ['needs_password', true],
    ['extracting', true],
    ['filtering', false],
    ['done', false],
    ['failed', false],
  ])('cancel is %s for %s', (status, expected) => {
    expect(canCancelJob(status)).toBe(expected)
  })

  it.each<[string | undefined, boolean]>([
    ['no_space', true],
    ['archive_corrupt', true],
    ['crc_mismatch', true],
    ['legacy_volume', false],
    ['no_video', false],
    [undefined, false],
  ])('retryability follows the server reason %s', (error, expected) => {
    expect(canRetryJob(error as UploadJob['error'])).toBe(expected)
  })

  it('counts supported multipart volumes in the browser', () => {
    const totals = multipartPartsTotal([
      new File([], 'movie.part1.rar'),
      new File([], 'movie.part2.rar'),
      new File([], 'season.7z.001'),
      new File([], 'season.7z.002'),
      new File([], 'legacy.r00'),
    ])
    expect(totals.get('movie')).toBe(2)
    expect(totals.get('season')).toBe(2)
    expect(totals.has('legacy')).toBe(false)
  })

  it('labels a stalled server upload as interrupted and keeps extraction progress', () => {
    const local: LocalUpload[] = []
    const rows = buildUploadRows(
      local,
      [job('uploading'), job('extracting')],
      Date.parse('2026-01-01T00:01:00Z'),
    )
    expect(rows.find((row) => row.jobId === 'uploading')?.phase).toBe('interrupted')
    expect(rows.find((row) => row.jobId === 'extracting')).toEqual(
      expect.objectContaining({ phase: 'extracting', progress: 34 }),
    )
  })
})
