import { QueryClient } from '@tanstack/react-query'

import {
  invalidateCatalogQueries,
  liveQueryOptions,
  scheduleCatalogInvalidation,
  unassignedKey,
} from './queries'

describe('consultas de catalogo', () => {
  it('no sondea vistas actualizadas por eventos en vivo', () => {
    expect(liveQueryOptions).toEqual({
      staleTime: 0,
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

  it('absorbe errores de refetch al invalidar el catálogo', async () => {
    const client = new QueryClient()
    const invalidate = vi
      .spyOn(client, 'invalidateQueries')
      .mockRejectedValue(new Error('notFound'))

    expect(() => {
      invalidateCatalogQueries(client)
    }).not.toThrow()
    await Promise.resolve()
    expect(invalidate).toHaveBeenCalledTimes(3)
  })
})
