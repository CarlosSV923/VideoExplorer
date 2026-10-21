import type { ApiClient } from '@/shared/api/client'
import { unwrap } from '@/shared/api/result'

import type { MetadataPorts } from '../../application/ports'
import type { ImageKind } from '../../domain/types'

export function createMetadataHttp(client: ApiClient, baseUrl = '/api'): MetadataPorts {
  return {
    searchTitles: (query, year) =>
      unwrap(
        client.GET('/titles/search', {
          params: { query: { query, ...(year === undefined ? {} : { year }) } },
        }),
      ),
    loadTitleMetadata: (titleId) =>
      unwrap(client.GET('/library/titles/{id}/metadata', { params: { path: { id: titleId } } })),
    refreshTitleMetadata: (titleId) =>
      unwrap(
        client.POST('/library/titles/{id}/metadata/refresh', { params: { path: { id: titleId } } }),
      ),
    imageUrl: (kind: ImageKind, file: string) =>
      `${baseUrl}/images/${kind}/${encodeURIComponent(file)}`,
  }
}
