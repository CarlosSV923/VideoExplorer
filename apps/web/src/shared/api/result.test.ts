import { AppError } from '@/shared/kernel/errors'

import { problemError, unwrap } from './result'

describe('API error mapping', () => {
  it.each<[number, string, string | undefined]>([
    [401, 'unauthorized', undefined],
    [404, 'notFound', undefined],
    [409, 'conflict', 'plan_changed'],
    [422, 'validation', undefined],
    [429, 'rateLimited', undefined],
    [507, 'insufficientStorage', 'ERR_INSUFFICIENT_STORAGE'],
    [503, 'server', undefined],
  ])('maps %s to %s', (status, kind, code) => {
    const error = problemError(status, {
      code,
      detail: 'technical English detail must not become the UI message',
    })

    expect(error).toBeInstanceOf(AppError)
    expect(error.kind).toBe(kind)
    expect(error.code).toBe(code)
    expect(error.message).not.toContain('technical English')
  })

  it('maps a rejected fetch to a network error', async () => {
    await expect(unwrap(Promise.reject(new TypeError('connection refused')))).rejects.toMatchObject(
      { kind: 'network', message: 'network' },
    )
  })
})
