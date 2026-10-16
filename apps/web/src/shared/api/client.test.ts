import { createApiClient, type ApiSchemas } from './client'

const json = (body: unknown, status: number, contentType = 'application/json') =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': contentType } })

describe('createApiClient', () => {
  it.each<[string, ApiSchemas['Health']]>([
    ['ok', { status: 'ok', checks: [], jellyfin: { state: 'unconfigured' } }],
    [
      'degraded',
      {
        status: 'degraded',
        checks: [
          {
            name: 'library',
            ok: false,
            code: 'library_not_writable',
            path: '/library',
            uid: 1000,
            gid: 1000,
            message: 'not writable',
          },
          { name: 'ffprobe', ok: true },
        ],
        jellyfin: { state: 'error' },
      },
    ],
    [
      'degraded by a missing program',
      {
        status: 'degraded',
        checks: [
          { name: '7zz', ok: false, code: 'sevenzip_missing', message: '7zz is not in PATH' },
        ],
        jellyfin: { state: 'unconfigured' },
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

  it('sends the shared password to /auth/login and types the 204 answer', async () => {
    const fetchMock = vi.fn<typeof fetch>(() =>
      Promise.resolve(new Response(null, { status: 204 })),
    )
    const client = createApiClient({ baseUrl: 'http://localhost/api', fetch: fetchMock })

    const { response, error } = await client.POST('/auth/login', {
      body: { password: 'secret' },
    })

    expect(response.status).toBe(204)
    expect(error).toBeUndefined()
    const request = fetchMock.mock.calls[0]?.[0] as Request
    expect(request.url).toBe('http://localhost/api/auth/login')
    expect(request.method).toBe('POST')
    await expect(request.json()).resolves.toEqual({ password: 'secret' })
  })

  it('types the 429 of a throttled login as a problem', async () => {
    const problem = {
      title: 'Too Many Requests',
      status: 429,
      detail: 'Too many attempts. Wait a moment and try again.',
    }
    const fetchMock = vi.fn<typeof fetch>(() =>
      Promise.resolve(json(problem, 429, 'application/problem+json')),
    )
    const client = createApiClient({ baseUrl: 'http://localhost/api', fetch: fetchMock })

    const { error } = await client.POST('/auth/login', { body: { password: 'x' } })

    expect(error).toEqual(problem)
  })

  it('types the 401 of /auth/session without a session as a problem', async () => {
    const problem = { title: 'Unauthorized', status: 401, detail: 'Sign in to continue.' }
    const fetchMock = vi.fn<typeof fetch>(() =>
      Promise.resolve(json(problem, 401, 'application/problem+json')),
    )
    const client = createApiClient({ baseUrl: 'http://localhost/api', fetch: fetchMock })

    const { data, error, response } = await client.GET('/auth/session')

    expect(response.status).toBe(401)
    expect(data).toBeUndefined()
    expect(error).toEqual(problem)
  })
})
