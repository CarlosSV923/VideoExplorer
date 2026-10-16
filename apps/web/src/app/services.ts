import type { CatalogPorts } from '@/modules/catalog/application/ports'
import { createCatalogHttp } from '@/modules/catalog/infrastructure/http/catalogHttp'
import type { IdentityPorts } from '@/modules/identity/application/ports'
import { createIdentityHttp } from '@/modules/identity/infrastructure/http/identityHttp'
import type { IngestionPorts } from '@/modules/ingestion/application/ports'
import { createIngestionHttp } from '@/modules/ingestion/infrastructure/http/ingestionHttp'
import type { MediaServersPorts } from '@/modules/mediaservers/application/ports'
import { createMediaServersHttp } from '@/modules/mediaservers/infrastructure/http/mediaServersHttp'
import type { MetadataPorts } from '@/modules/metadata/application/ports'
import { createMetadataHttp } from '@/modules/metadata/infrastructure/http/metadataHttp'
import type { SystemPorts } from '@/modules/system/application/ports'
import { createSystemHttp } from '@/modules/system/infrastructure/http/systemHttp'
import { createApiClient } from '@/shared/api/client'

import type { DataSource } from './config'

export interface Services {
  catalog: CatalogPorts
  identity: IdentityPorts
  ingestion: IngestionPorts
  mediaServers: MediaServersPorts
  metadata: MetadataPorts
  system: SystemPorts
}

export function createHttpServices(): Services {
  const client = createApiClient()
  return {
    catalog: createCatalogHttp(client),
    identity: createIdentityHttp(client),
    ingestion: createIngestionHttp(client),
    mediaServers: createMediaServersHttp(client),
    metadata: createMetadataHttp(client),
    system: createSystemHttp(client),
  }
}

/** Demo adapters are introduced in phase 11; phase 10 composes HTTP only. */
export function createServices(source: DataSource): Services {
  switch (source) {
    case 'http':
    case 'demo':
      return createHttpServices()
  }
}
