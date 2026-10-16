import { QueryClient } from '@tanstack/react-query'

import { liveQueryOptions, scheduleCatalogInvalidation, unassignedKey } from './queries'

describe('consultas de catalogo', () => {
  it('no sondea vistas actualizadas por eventos en vivo', () => {
    expect(liveQueryOptions).toEqual({
      refetchInterval: false,
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
    })
  })

  it('agrupa invalidaciones síncronas de una ráfaga SSE', async () => {
    const client = new QueryClient()
    const invalidate = vi.spyOn(client, 'invalidateQueries')

    scheduleCatalogInvalidation(client, [unassignedKey])
    scheduleCatalogInvalidation(client, [unassignedKey])

    expect(invalidate).not.toHaveBeenCalled()
    await Promise.resolve()
    expect(invalidate).toHaveBeenCalledTimes(1)
    expect(invalidate).toHaveBeenCalledWith({ queryKey: unassignedKey })
  })
})
