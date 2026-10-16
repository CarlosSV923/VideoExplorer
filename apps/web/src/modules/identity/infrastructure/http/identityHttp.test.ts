import { createApiClient } from '@/shared/api/client'

import { createIdentityHttp } from './identityHttp'

describe('identity HTTP adapter', () => {
  it('maps login through the typed API client and sends the password', async () => {
    const fetchMock = vi.fn<typeof fetch>(() =>
      Promise.resolve(new Response(null, { status: 204 })),
    )
    const ports = createIdentityHttp(
      createApiClient({ baseUrl: 'http://localhost/api', fetch: fetchMock }),
    )

    await ports.login('secret')

    const request = fetchMock.mock.calls[0]?.[0] as Request
    expect(request.url).toBe('http://localhost/api/auth/login')
    expect(request.method).toBe('POST')
    await expect(request.json()).resolves.toEqual({ password: 'secret' })
  })
})
