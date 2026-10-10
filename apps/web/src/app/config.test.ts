import { resolveDataSource } from './config'

describe('resolveDataSource', () => {
  it('selects the demo adapters only when explicitly requested', () => {
    expect(resolveDataSource('demo')).toBe('demo')
  })

  it.each([undefined, '', 'http', 'api', 'DEMO', 'mock'])('falls back to HTTP for %j', (raw) => {
    expect(resolveDataSource(raw)).toBe('http')
  })
})
