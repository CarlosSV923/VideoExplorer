import { createApiClient, type ApiSchemas } from './client'

const json = (body: unknown, status: number, contentType = 'application/json') =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': contentType } })

describe('createApiClient', () => {
  it.each<[string, ApiSchemas['Health']]>([
    ['ok', { status: 'ok', checks: [] }],
    [
      'degraded',
      {
        status: 'degraded',
        checks: [{ name: 'library', ok: false, message: 'not writable' }],
      },
    ],
  ])('types the /health answer when the status is %s', async (_label, body) => {
    const fetchMock = vi.fn<typeof fetch>(() => Promise.resolve(json(body, 200)))
    const client = createApiClient({ baseUrl: 'http://localhost/api', fetch: fetchMock })

    const { data, error } = await client.GET('/health')

    expect(error).toBeUndefined()
    expect(data).toEqual(body)
    const request = fetchMock.mock.calls[0]?.[0] as Request
    expect(request.url).toBe('http://localhost/api/health')
  })

  it('exposes RFC 9457 problems as typed errors', async () => {
    const problem = { title: 'Not Found', status: 404 }
    const fetchMock = vi.fn<typeof fetch>(() =>
      Promise.resolve(json(problem, 404, 'application/problem+json')),
    )
    const client = createApiClient({ baseUrl: 'http://localhost/api', fetch: fetchMock })

    const { data, error, response } = await client.GET('/health')

    expect(response.status).toBe(404)
    expect(data).toBeUndefined()
    expect(error).toEqual(problem)
  })
})
